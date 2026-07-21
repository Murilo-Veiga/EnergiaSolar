-- Seed do usuário admin padrão pra instalação nova (sem cadastro público).
-- Login: admin / password — troque a senha em "Minha conta" assim que
-- entrar pela primeira vez.
--
-- WHERE NOT EXISTS em vez de ON CONFLICT (email) DO NOTHING de propósito:
-- username também é UNIQUE, então uma instalação que já tivesse alguém com
-- username 'admin' (e-mail diferente) batia numa constraint que o ON
-- CONFLICT (email) não cobre — erro não tratado no meio da migration,
-- deixando o banco "dirty".
INSERT INTO users (name, email, username, password_hash, is_admin)
SELECT 'Administrador', 'admin@example.com', 'admin',
       '$2b$10$jcd2YAlFb/dK1Rt30WY1IuqzEJRiztaOV1EyFXOzmBIuUxotg1g2y', true
WHERE NOT EXISTS (
    SELECT 1 FROM users WHERE email = 'admin@example.com' OR username = 'admin'
);
