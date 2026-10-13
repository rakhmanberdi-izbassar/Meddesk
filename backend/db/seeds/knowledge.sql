-- Synthetic troubleshooting guides. Executed only by the development-gated Go seed.
-- Existing content, publication state and counters are deliberately preserved.
INSERT INTO knowledge_tags (name) VALUES ('NCALayer'),('ЭЦҚ'),('Принтер'),('Желі')
ON CONFLICT (lower(btrim(name))) DO NOTHING;

WITH new_articles AS (
INSERT INTO knowledge_articles
    (slug,title,description,category_id,symptoms,solution_steps,author_id,is_published,published_at)
SELECT seed.slug,seed.title,seed.description,c.id,seed.symptoms,seed.steps::jsonb,u.id,true,clock_timestamp()
FROM (VALUES
    ('ncalayer-iske-qosu','NCALayer бағдарламасын іске қосу',
     'NCALayer қосылмаған кезде ЭЦҚ арқылы кіруді тексеру. Запуск NCALayer для входа с ЭЦП.',
     'NCALayer іске қосылмаған. ЭЦҚ арқылы кіру орындалмайды. NCALayer не запущен, вход с ЭЦП не работает.',
     '["NCALayer компьютерде орнатылғанын тексеріңіз. Проверьте, установлено ли приложение.","Бастау мәзірінен NCALayer ашыңыз. Откройте NCALayer через меню Пуск.","Тапсырмалар жолағындағы қолданба белгішесін тексеріңіз. Убедитесь, что приложение запущено.","Қолданбаны қайта іске қосып, браузер бетін жаңартыңыз. Перезапустите приложение и обновите страницу.","Мәселе сақталса, IT маманына хабарласыңыз. Если проблема остаётся, обратитесь к IT специалисту."]',
     'ncalayer-eds'),
    ('printer-kezek-tekseru','Принтердің басып шығару кезегін тексеру',
     'Басып шығару орындалмаған кезде принтер мен кезекті тексеру. Проверка очереди печати принтера.',
     'Принтер басып шығармайды, құжат кезекте тұр. Принтер не печатает, документы зависли в очереди печати.',
     '["Принтер қосылғанын және қағаз бар екенін тексеріңіз. Проверьте питание и наличие бумаги.","Windows параметрлерінен дұрыс принтер таңдалғанын тексеріңіз. Убедитесь, что выбран нужный принтер.","Принтердің басып шығару кезегін ашып, күйін қараңыз. Откройте очередь печати и проверьте состояние.","Өзіңіз жіберген қайталанған тапсырманы ғана жойып, шағын тест құжатын басып көріңіз. Отмените только свою повторную задачу и проверьте небольшим тестовым документом.","Қате сақталса, IT маманына хабарласыңыз. Если ошибка сохраняется, обратитесь к IT специалисту."]',
     'printers'),
    ('internet-qosylym-tekseru','Интернет қосылымын тексеру',
     'Желі кабелі мен қосылым күйін тексеру. Проверка подключения к сети и интернету.',
     'Интернетке қосылу жоқ, сайт ашылмайды. Нет подключения к интернету, сайт не открывается.',
     '["Желі белгішесінің күйін тексеріңіз. Проверьте значок подключения.","Желі кабелі компьютерге жалғанғанын тексеріңіз. Проверьте подключение сетевого кабеля.","Браузерде клиниканың рұқсат етілген басқа бетін ашып көріңіз. Откройте другую разрешённую страницу.","Өз жұмысыңызды сақтап, қажет болса компьютерді қайта іске қосыңыз. Сохраните работу и при необходимости перезапустите компьютер.","Желі әлі қосылмаса, IT маманына хабарласыңыз. Если соединения нет, обратитесь к IT специалисту."]',
     'network')
) AS seed(slug,title,description,symptoms,steps,category_slug)
JOIN knowledge_categories c ON c.slug=seed.category_slug AND c.is_active
JOIN users u ON u.email='engineer@meddesk.test' AND u.is_active
JOIN roles r ON r.id=u.role_id AND r.code='IT_ENGINEER'
ON CONFLICT (slug) DO NOTHING
RETURNING id,slug
)
INSERT INTO article_tags (article_id,tag_id)
SELECT a.id,t.id FROM new_articles a JOIN knowledge_tags t ON
    (a.slug='ncalayer-iske-qosu' AND lower(t.name) IN ('ncalayer','эцқ'))
 OR (a.slug='printer-kezek-tekseru' AND lower(t.name)='принтер')
 OR (a.slug='internet-qosylym-tekseru' AND lower(t.name)='желі')
ON CONFLICT DO NOTHING;
