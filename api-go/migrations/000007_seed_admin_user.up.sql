-- Seed do usuário admin padrão pra instalação nova (sem cadastro público).
-- Login: admin / password — troque a senha em "Minha conta" assim que
-- entrar pela primeira vez.
INSERT INTO users (name, email, username, password_hash, is_admin)
VALUES (
    'Administrador',
    'admin@example.com',
    'admin',
    '$2b$10$jcd2YAlFb/dK1Rt30WY1IuqzEJRiztaOV1EyFXOzmBIuUxotg1g2y',
    true
)
ON CONFLICT (email) DO NOTHING;
