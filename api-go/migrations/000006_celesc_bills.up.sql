-- Faturas da Celesc importadas via upload de PDF (parser em
-- internal/celesc) — alimenta a comparação consumo x geração e a tarifa
-- efetiva usada em summary/day-status/history (ver README > "Fatura
-- Celesc"). Cada linha é 1 mês de referência de 1 unidade consumidora.
CREATE TABLE celesc_bills (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    consumer_unit_id   uuid NOT NULL REFERENCES consumer_units(id) ON DELETE CASCADE,
    referencia_ano     int NOT NULL,
    referencia_mes     int NOT NULL,
    consumo_kwh        double precision NOT NULL,
    total_pagar_brl    double precision,
    dias_faturados     int,
    bandeira           text,
    bandeira_valor_kwh double precision,
    titular            text,
    source             text NOT NULL, -- 'fatura' | 'backfill_historico'
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (consumer_unit_id, referencia_ano, referencia_mes)
);
CREATE INDEX celesc_bills_consumer_unit_id_idx
    ON celesc_bills(consumer_unit_id, referencia_ano DESC, referencia_mes DESC);
