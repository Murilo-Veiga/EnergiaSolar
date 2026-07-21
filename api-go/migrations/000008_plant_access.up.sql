-- Compartilhamento somente-leitura de usinas: o dono (plants.user_id)
-- concede acesso de visualização a outra conta sem transferir a
-- propriedade nem exigir privilégio de admin. Toda escrita (editar usina,
-- gerenciar credenciais de inversor, lançar anotação) continua exigindo
-- ser o dono — ver authorizePlant vs. authorizePlantView em plant_handlers.go.
CREATE TABLE plant_access (
    plant_id   uuid NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (plant_id, user_id)
);
CREATE INDEX plant_access_user_id_idx ON plant_access(user_id);
