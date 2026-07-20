-- Configurações globais do sistema — não dependem de usuário nem de usina
-- (ex.: URL padrão das integrações Huawei/FoxESS, intervalo do worker de
-- coleta). Tabela singleton: sempre 1 linha só (id fixo em true).
CREATE TABLE system_settings (
    id                       boolean PRIMARY KEY DEFAULT true,
    huawei_base_url          text NOT NULL DEFAULT '',
    foxess_base_url          text NOT NULL DEFAULT '',
    worker_interval_minutes  integer NOT NULL DEFAULT 30,
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT system_settings_singleton CHECK (id)
);

INSERT INTO system_settings (id) VALUES (true);
