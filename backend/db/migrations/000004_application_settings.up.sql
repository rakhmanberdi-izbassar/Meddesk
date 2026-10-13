CREATE TABLE application_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    clinic_name text NOT NULL DEFAULT 'MedDesk' CHECK (length(btrim(clinic_name)) BETWEEN 1 AND 150 AND clinic_name !~ '[[:cntrl:]]'),
    default_ticket_priority text NOT NULL DEFAULT 'MEDIUM' CHECK (default_ticket_priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    reporting_timezone text NOT NULL DEFAULT 'Asia/Almaty' CHECK (length(reporting_timezone) BETWEEN 1 AND 100),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO application_settings (singleton) VALUES (true);
CREATE TRIGGER application_settings_updated_at BEFORE UPDATE ON application_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE INDEX tickets_created_analytics_idx ON tickets (created_at);
