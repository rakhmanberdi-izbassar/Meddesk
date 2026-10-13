-- Synthetic reference data only. Run via the APP_ENV=development-gated seed job.
-- Account/article provisioning is handled by the explicit Go development seed.
BEGIN;
INSERT INTO departments (name) VALUES
    ('Демо: қабылдау бөлімі'),
    ('Демо: әкімшілік'),
    ('Демо: IT бөлімі')
ON CONFLICT (lower(btrim(name))) DO NOTHING;

INSERT INTO knowledge_categories (slug, name_kk, name_ru, description) VALUES
    ('ncalayer-eds', 'NCALayer және ЭЦҚ', 'NCALayer и ЭЦП', 'Цифрлық қолтаңбаға қатысты техникалық нұсқаулықтар'),
    ('printers', 'Принтерлер', 'Принтеры', 'Басып шығару және кезек мәселелері'),
    ('network', 'Желі және интернет', 'Сеть и интернет', 'Желі қосылымын тексеру'),
    ('windows', 'Windows', 'Windows', 'Жұмыс станциясын баптау'),
    ('browsers-office', 'Браузерлер және Office', 'Браузеры и Office', 'Браузерлер мен кеңсе бағдарламалары')
ON CONFLICT (slug) DO NOTHING;
COMMIT;
