# Knowledge Base content

`knowledge-categories.json` contains 13 initial Kazakh/Russian category records in the implemented `CategoryInput` API format. Descriptions identify common issue types. Records contain no credentials, patient information or private certificates.

This file is content preparation, not a database migration or automatic production seed. Before importing through the authenticated administrator interface/API, list all existing categories, match stable slugs and review legacy combined categories such as `browsers-office`. Add only missing categories; do not silently overwrite, deactivate or duplicate existing clinic taxonomy. Creating these records should use normal administrator authorization and transactional audit behavior. A local JSON file alone does not mean production records have been created.

## Applied categories

On 2026-10-26, the 13 records were created through the authenticated administrator UI at `https://meddeskv2.up.railway.app/admin/categories`. The initial category list was empty. Each save returned confirmation; after reloading the page, all 13 slugs and Kazakh/Russian names were verified and every category was active. This operation created categories only, not troubleshooting articles. The signed-in operator's credentials were not stored in these files.

## Applied articles

On 2026-10-26, `knowledge-articles.json` was published through the authenticated staff editor: 26 text guides, two for each of the 13 categories. Titles/steps are Kazakh; Russian issue phrases and multilingual tags support search. Records include symptoms, 5–6 ordered checks, an IT escalation boundary and source URLs where vendor guidance is used. Clinic-specific records describe general technical triage rather than inventing a workflow for an unidentified clinical system. No videos, patient records, credentials or private key material were uploaded.

`knowledge-articles-published.json` records the created slugs and article detail URLs. This is a publication receipt, not an automatic reimport job. Do not create these slugs again blindly. Each save was verified on the detail page; after reloading the public-content knowledge list, all 26 expected titles were verified across three pages. Actual UI searches for `NCALayer`, `не печатает` and `құжат` returned the expected guides. No helpful/not-helpful outcome was submitted during verification. Checks used the signed-in administrator session; a separate employee-session acceptance run was not performed in this content turn.
