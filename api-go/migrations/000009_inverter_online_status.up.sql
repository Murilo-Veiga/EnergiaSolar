ALTER TABLE inverter_status ADD COLUMN online boolean;
ALTER TABLE inverter_status ADD COLUMN last_online_at timestamptz;
