CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Config / cadastro --------------------------------------------------------

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE plants (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                 text NOT NULL,
    lat                  double precision,
    lon                  double precision,
    installed_power_kwp  double precision NOT NULL DEFAULT 0,
    timezone             text NOT NULL DEFAULT 'America/Sao_Paulo',
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plants_user_id_idx ON plants(user_id);

CREATE TABLE inverter_credentials (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id                uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    brand                   text NOT NULL CHECK (brand IN ('huawei', 'foxess')),
    enabled                 boolean NOT NULL DEFAULT true,
    credentials_encrypted   bytea NOT NULL,
    discovered_station_code text,
    discovered_dev_dn       text,
    discovered_device_sn    text,
    last_success_at         timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (plant_id, brand)
);
CREATE INDEX inverter_credentials_plant_id_idx ON inverter_credentials(plant_id);

CREATE TABLE consumer_units (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id   uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    uc_number  text NOT NULL,
    label      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (plant_id, uc_number)
);

-- Série temporal -------------------------------------------------------------
-- Substitui os measurements do InfluxDB (ver README > "Estrutura do projeto").

CREATE TABLE plant_status (
    plant_id              uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    recorded_at           timestamptz NOT NULL,
    instantaneous_power_kw double precision NOT NULL,
    installed_power_kwp   double precision,
    has_alarm             boolean NOT NULL DEFAULT false,
    alarm_detail          text
);
CREATE INDEX plant_status_plant_id_recorded_at_idx ON plant_status(plant_id, recorded_at DESC);

CREATE TABLE inverter_status (
    plant_id      uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    inverter      text NOT NULL,
    recorded_at   timestamptz NOT NULL,
    power_kw      double precision,
    day_kwh       double precision,
    temperature_c double precision
);
CREATE INDEX inverter_status_plant_id_recorded_at_idx ON inverter_status(plant_id, inverter, recorded_at DESC);

CREATE TABLE daily_generation (
    plant_id      uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    day           date NOT NULL,
    generated_kwh double precision NOT NULL,
    PRIMARY KEY (plant_id, day)
);

CREATE TABLE collector_health (
    plant_id             uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    inverter             text NOT NULL,
    recorded_at          timestamptz NOT NULL,
    consecutive_failures int NOT NULL DEFAULT 0,
    last_error           text
);
CREATE INDEX collector_health_plant_id_recorded_at_idx ON collector_health(plant_id, inverter, recorded_at DESC);

CREATE TABLE consumption (
    plant_id       uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    uc             text NOT NULL,
    recorded_at    timestamptz NOT NULL,
    source         text NOT NULL DEFAULT 'fatura',
    consumed_kwh   double precision,
    total_value_brl double precision,
    reference      text,
    due_date       date,
    bandeira       text
);
CREATE INDEX consumption_plant_id_uc_recorded_at_idx ON consumption(plant_id, uc, recorded_at DESC);

CREATE TABLE annotation (
    plant_id uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    day      date NOT NULL,
    note     text NOT NULL,
    PRIMARY KEY (plant_id, day)
);
