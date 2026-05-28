BEGIN;

ALTER TABLE tasks
DROP CONSTRAINT tasks_type_check;

ALTER TABLE tasks
ADD CONSTRAINT tasks_type_check
CHECK (
    type IN (
        'http_call',
        'shell',
        'grpc',
        'kafka',
        'email'
    )
);

COMMIT;