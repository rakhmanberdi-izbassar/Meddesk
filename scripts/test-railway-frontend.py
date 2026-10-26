#!/usr/bin/env python3
"""Exercise the Railway frontend image using a disposable synthetic API and network."""
import json, re, subprocess, tempfile, time, urllib.request, urllib.error, uuid
from pathlib import Path
image='meddesk-web-railway:local'
tag='meddesk-web-check-'+uuid.uuid4().hex[:10]
containers=[]
def docker(*args, **kw):
    return subprocess.check_output(['docker',*args], text=True, stderr=subprocess.STDOUT, **kw).strip()
def fetch(path, data=None, headers=None):
    req=urllib.request.Request(base+path, data=data, headers=headers or {})
    try:
        with urllib.request.urlopen(req,timeout=4) as r: return r.status,r.headers,r.read()
    except urllib.error.HTTPError as r: return r.code,r.headers,r.read()
with tempfile.TemporaryDirectory(prefix=tag) as tmp:
    folder=Path(tmp)
    folder.chmod(0o755)
    media=folder/'sample.bin'
    payload=bytes(range(256))*1024
    media.write_bytes(payload)
    media.chmod(0o644)
    conf=folder/'mock.conf'
    conf.write_text('''pid /tmp/mock.pid;
error_log /dev/null;
events {}
http {
 access_log off;
 client_body_temp_path /tmp/mock_client;
 proxy_temp_path /tmp/mock_proxy;
 fastcgi_temp_path /tmp/mock_fastcgi;
 uwsgi_temp_path /tmp/mock_uwsgi;
 scgi_temp_path /tmp/mock_scgi;
 server { listen 8081;
  location = /api/v1/ready { default_type application/json; return 200 '{"status":"ready"}'; }
  location = /api/v1/check { default_type text/plain; return 200 "$request_method|$request_uri|$http_origin|$http_cookie|$http_x_csrf_token"; }
  location = /api/v1/media/sample { alias /tmp/sample.bin; }
  location /api/ { default_type application/json; return 404 '{"error":{"code":"not_found"}}'; }
 }
}''')
    conf.chmod(0o644)
    docker('network','create',tag)
    try:
        fixture=docker('run','--rm','-d','--name',tag+'-api','--network',tag,'--network-alias','meddesk-fixture','-v',str(conf)+':/tmp/mock.conf:ro','-v',str(media)+':/tmp/sample.bin:ro','--entrypoint','nginx',image,'-c','/tmp/mock.conf','-g','daemon off;')
        containers.append(fixture)
        web=docker('run','--rm','-d','--name',tag+'-web','--network',tag,'-p','127.0.0.1::8080','-e','BACKEND_HOST=meddesk-fixture','-e','BACKEND_PORT=8081',image)
        containers.append(web)
        port=docker('port',web,'8080/tcp').rsplit(':',1)[1]
        base='http://127.0.0.1:'+port
        for attempt in range(30):
            try:
                if fetch('/healthz')[0]==200: break
            except OSError: pass
            time.sleep(.2)
        else: raise AssertionError('gateway did not start: '+docker('logs',web))
        for path in ['/', '/login', '/knowledge/articles/synthetic-id']:
            status,headers,body=fetch(path)
            assert status==200 and b'<html' in body.lower(), path
        status,headers,body=fetch('/login')
        asset=re.search(rb'src="(/assets/[^\"]+\.js)"',body).group(1).decode()
        assert fetch(asset)[0]==200
        assert fetch('/assets/missing.js')[0]==404
        status,headers,body=fetch('/api/v1/ready')
        assert status==200, (status, body.decode(), docker('logs',fixture), docker('logs',web))
        assert json.loads(body)=={'status':'ready'}
        status,headers,body=fetch('/api/v1/missing')
        assert status==404 and json.loads(body)['error']['code']=='not_found'
        status,headers,body=fetch('/api/v1/check?search=NCALayer%20test', data=b'{}', headers={'Content-Type':'application/json','Origin':'https://example.test','Cookie':'synthetic_cookie=value','X-CSRF-Token':'synthetic-token'})
        assert status==200 and body.decode()=='POST|/api/v1/check?search=NCALayer%20test|https://example.test|synthetic_cookie=value|synthetic-token'
        status,headers,body=fetch('/api/v1/media/sample',headers={'Range':'bytes=1024-1039'})
        assert status==206 and body==payload[1024:1040] and headers['Content-Range']=='bytes 1024-1039/'+str(len(payload))
        assert headers['X-Content-Type-Options']=='nosniff'
        assert "connect-src 'self'" in headers['Content-Security-Policy']
        assert docker('exec',web,'id','-u')!='0'
        invalid=subprocess.run(['docker','run','--rm','-e','BACKEND_HOST=bad;directive','-e','BACKEND_PORT=8081',image],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        assert invalid.returncode!=0 and 'BACKEND_HOST must be' in invalid.stdout
        absent=subprocess.run(['docker','run','--rm','-e','BACKEND_HOST=meddesk-fixture','-e','BACKEND_PORT=65536',image],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        assert absent.returncode!=0 and 'BACKEND_PORT must be' in absent.stdout
        print('PASS: non-root startup, SPA/deep links/assets, API readiness and 404, POST/query/Origin/cookie/CSRF forwarding, media ranges/security headers, invalid configuration rejection.')
    finally:
        for container in reversed(containers):
            subprocess.run(['docker','stop','--time','2',container],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        subprocess.run(['docker','network','rm',tag],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
