-- Level 6: state layer buat go-api - tabel users, konsisten dengan data
-- dummy yang dipakai sejak Level 1 (Alice, Bob, Charlie).
CREATE TABLE IF NOT EXISTS users (
    id   SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO users (id, name) VALUES
    (1, 'Alice'),
    (2, 'Bob'),
    (3, 'Charlie')
ON CONFLICT (id) DO NOTHING;

-- reset sequence biar insert berikutnya (kalau ada) mulai dari id 4
SELECT setval('users_id_seq', (SELECT MAX(id) FROM users));
