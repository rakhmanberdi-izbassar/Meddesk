# Article localization

Kazakh content remains in knowledge_articles. Optional complete Russian content is stored in knowledge_article_translations, introduced by migration 000005. Categories already have name_kk/name_ru.

The API returns original content plus translation_ru (title, description, symptoms, solution_steps). The frontend selects content using the interface locale in article cards, detail pages and ticket suggestions. Missing translations fall back to Kazakh; article details explicitly show this fallback. Switching language does not record another view. Search covers original content and indexed Russian translations. Tags and media are shared.

Editors enter both versions in the existing article editor. Partial Russian translations are rejected. Omitting translation_ru on an update preserves the existing translation for older clients. Article version checks and transactional audit logging also cover translation changes. Translations are editorial content; changing Kazakh text requires reviewing Russian text as well.

## Deployment

Back up the database, then apply migration 000005 using the existing migration workflow. Deploy backend and frontend from this revision. This feature requires migration 5; current readiness also requires subsequent migrations (currently version 6). This migration preserves existing articles and does not invent translations: existing articles need Russian content entered through the editor. Existing content JSON files are Kazakh source material, not a translation import.

Do not roll back migration 5 on production: its down migration deletes Russian translations. Test rollbacks only on disposable databases.
