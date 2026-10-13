#!/usr/bin/env python3
"""Exercise development sessions against the local gateway without printing secrets."""
import http.cookiejar
import io
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile


def main():
    configuration = {}
    for line in (Path(__file__).resolve().parent.parent / ".env").read_text().splitlines():
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            configuration[key] = value
    if configuration.get("APP_ENV") != "development":
        raise RuntimeError("Authentication smoke requires APP_ENV=development.")
    base = os.environ.get("MEDDESK_URL", "http://127.0.0.1:8080").rstrip("/")
    address = urllib.parse.urlparse(base)
    if (address.scheme not in ("http", "https") or address.hostname not in
            ("localhost", "127.0.0.1", "::1") or address.username or address.password or
            address.path or address.query or address.fragment):
        raise RuntimeError("Authentication smoke only sends demo credentials to a loopback origin.")

    def request(client, method, path, expected, payload=None, csrf=None, origin=True, binary=False):
        headers = {"Accept": "application/json"}
        if origin:
            headers["Origin"] = base
        if csrf:
            headers["X-CSRF-Token"] = csrf
        body = None
        if payload is not None:
            headers["Content-Type"] = "application/json"
            body = json.dumps(payload).encode()
        operation = urllib.request.Request(base + "/api/v1" + path, body, headers, method=method)
        try:
            response = client.open(operation, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            if response.status != expected:
                raise RuntimeError(f"{method} {path}: expected {expected}, received {response.status}.")
            content = response.read()
            if binary:
                return response.headers, content
            return json.loads(content) if content else None

    anonymous = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    request(anonymous, "GET", "/auth/me", 401)
    request(anonymous, "GET", "/users", 401)
    request(anonymous, "GET", "/knowledge/articles", 401)
    request(anonymous, "GET", "/tickets", 401)
    request(anonymous, "GET", "/tickets/export", 401)
    request(anonymous, "GET", "/settings", 401)
    request(anonymous, "GET", "/analytics", 401)
    request(anonymous, "GET", "/admin/audit", 401)
    print("PASS anonymous account and administration access denied")
    for label, email, role in (
        ("EMPLOYEE", "employee@meddesk.test", "EMPLOYEE"),
        ("ENGINEER", "engineer@meddesk.test", "IT_ENGINEER"),
        ("ADMIN", "admin@meddesk.test", "ADMIN"),
    ):
        password = configuration.get(f"DEMO_{label}_PASSWORD")
        if not password:
            raise RuntimeError("Demo passwords are missing; run make seed-accounts first.")
        cookies = http.cookiejar.CookieJar()
        client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))
        session = request(client, "POST", "/auth/login", 200, {"email": email, "password": password})
        if session["user"]["role"] != role:
            raise RuntimeError("Demo role differs from the seeded role; existing accounts are preserved.")
        current = request(client, "GET", "/auth/me", 200)
        if current["user"]["id"] != session["user"]["id"]:
            raise RuntimeError("Session identity mismatch.")
        if len(list(cookies)) != 2 or any(cookie.path != "/api/v1" or
                not cookie.has_nonstandard_attr("HttpOnly") for cookie in cookies):
            raise RuntimeError("Expected two scoped HttpOnly cookies.")
        expected = 200 if role == "ADMIN" else 403
        request(client, "GET", "/users?limit=5&offset=0", expected)
        request(client, "GET", "/departments?limit=5&offset=0", expected)
        guides = request(client, "GET", "/knowledge/articles?limit=5", 200)
        if any(not guide["is_published"] or not guide["category_active"] for guide in guides["items"]):
            raise RuntimeError("Default guide list included unpublished or inactive-category content.")
        request(client, "GET", "/knowledge/categories?is_active=true&limit=100", 200)
        request(client, "GET", "/knowledge/articles?published=all", 403 if role == "EMPLOYEE" else 200)
        for search in ("NCALayer", "ЭЦҚ", "принтеры"):
            matches = request(client, "GET", "/knowledge/articles?" + urllib.parse.urlencode({"q": search}), 200)
            if not matches["items"]:
                raise RuntimeError("Synthetic guide search returned no match; run the development seed.")
        guide = guides["items"][0]
        request(client, "GET", "/knowledge/articles/" + guide["id"], 200)
        request(client, "GET", "/knowledge/tags?limit=20", 200)
        tickets = request(client, "GET", "/tickets?limit=5", 200)
        if role == "EMPLOYEE" and any(ticket["requester_id"] != current["user"]["id"]
                                      for ticket in tickets["items"]):
            raise RuntimeError("Employee ticket list included another requester.")
        for ticket in tickets["items"]:
            detail = request(client, "GET", "/tickets/" + ticket["id"], 200)
            if role == "EMPLOYEE" and any(comment["is_internal"] for comment in detail["comments"]):
                raise RuntimeError("Employee ticket detail included an internal comment.")
        request(client, "GET", "/tickets/assignees", 403 if role == "EMPLOYEE" else 200)
        request(client, "GET", "/tickets/departments", 403 if role == "EMPLOYEE" else 200)
        if role == "EMPLOYEE":
            request(client, "GET", "/tickets/export?format=xlsx", 403)
            request(client, "GET", "/tickets/export?format=csv", 403)
        else:
            # Select no real requests: verify deployed downloads without emitting or storing ticket data.
            export_query = urllib.parse.urlencode({"q": str(uuid.uuid4()), "locale": "kk"})
            for export_format, mime in (("xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"), ("csv", "text/csv")):
                headers, data = request(client, "GET", f"/tickets/export?{export_query}&format={export_format}", 200, binary=True)
                if (headers.get_content_type() != mime or
                        not re.fullmatch(r'attachment; filename="meddesk-tickets-\d{8}-\d{6}\.' + export_format + r'"', headers.get("Content-Disposition", "")) or
                        "no-store" not in headers.get("Cache-Control", "")):
                    raise RuntimeError("Ticket download headers are incorrect.")
                if export_format == "xlsx":
                    with zipfile.ZipFile(io.BytesIO(data)) as workbook:
                        if "xl/worksheets/sheet1.xml" not in workbook.namelist():
                            raise RuntimeError("Ticket Excel workbook is incomplete.")
                elif not data.startswith(b"\xef\xbb\xbf"):
                    raise RuntimeError("Ticket CSV is missing its UTF-8 BOM.")
            request(client, "GET", "/tickets/export?format=xlsx&limit=20", 400)
        suggestions = request(client, "GET", "/tickets/suggestions?q=NCALayer", 200)
        if not suggestions["items"] or any(not article["is_published"] or not article["category_active"]
                                            for article in suggestions["items"]):
            raise RuntimeError("Ticket suggestions did not use published active-category guides.")
        request(client, "POST", "/tickets", 403, {})
        request(client, "POST", "/media?purpose=TICKET_ATTACHMENT", 403, {})
        settings = request(client, "GET", "/settings", 200)["settings"]
        if set(settings) != {"clinic_name", "default_ticket_priority", "reporting_timezone", "version", "updated_at"}:
            raise RuntimeError("Application settings exposed unexpected fields.")
        expected_reporting = 403 if role == "EMPLOYEE" else 200
        report = request(client, "GET", "/analytics", expected_reporting)
        request(client, "GET", "/analytics/filters", expected_reporting)
        if expected_reporting == 200:
            summary = report["summary"]
            if summary["tickets_total"] != summary["tickets_open"] + summary["tickets_resolved"] or summary["tickets_closed"] > summary["tickets_resolved"]:
                raise RuntimeError("Analytics ticket totals are inconsistent.")
            rate = summary["self_service_rate"]
            if (summary["feedback_attempts"] == 0 and rate is not None) or (rate is not None and not 0 <= rate <= 1):
                raise RuntimeError("Analytics self-service rate is inconsistent.")
            request(client, "GET", "/analytics?from=2026-10-12&to=2026-10-11", 400)
        expected_audit = 200 if role == "ADMIN" else 403
        request(client, "GET", "/admin/audit?limit=5", expected_audit)
        request(client, "GET", "/admin/audit/filters", expected_audit)
        request(client, "PATCH", "/settings", 403, {"version": settings["version"], "clinic_name": settings["clinic_name"]})
        request(client, "POST", "/knowledge/articles", 403, {})
        request(client, "POST", "/media?purpose=ARTICLE_IMAGE", 403, {})
        request(client, "PATCH", "/profile", 403, {"display_name": "Smoke"})
        request(client, "POST", "/auth/logout", 403, csrf=current["csrf_token"], origin=False)
        request(client, "POST", "/auth/logout", 204, csrf=current["csrf_token"])
        request(client, "GET", "/auth/me", 401)
        if list(cookies):
            raise RuntimeError("Logout did not clear authentication cookies.")
        print(f"PASS {role}: login, RBAC, CSRF, Origin, knowledge/tickets/export, analytics/audit/settings permissions and logout")


if __name__ == "__main__":
    try:
        main()
    except RuntimeError as failure:
        print(f"FAIL {failure}", file=sys.stderr)
        sys.exit(1)
    except Exception:
        # Network/configuration/parser exception details can contain credentials or tokens.
        print("FAIL authentication smoke; check gateway, local .env and development seed.", file=sys.stderr)
        sys.exit(1)
