ALTER TABLE sessions ADD COLUMN summary TEXT NOT NULL DEFAULT '';

UPDATE sessions
SET summary = COALESCE((
    SELECT substr(TRIM(json_extract(m.content, '$.text')), 1, 200)
    FROM messages m
    WHERE m.session_id = sessions.id
      AND m.role = 'user'
      AND json_extract(m.content, '$.text') IS NOT NULL
      AND TRIM(json_extract(m.content, '$.text')) != ''
    ORDER BY m.event_seq ASC, m.created_at ASC
    LIMIT 1
), '');
