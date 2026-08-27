CREATE TABLE changed_symbols (
    id BIGSERIAL PRIMARY KEY,
    changed_file_id BIGINT NOT NULL REFERENCES changed_files(id) ON DELETE CASCADE,
    symbol_name TEXT NOT NULL,
    symbol_kind TEXT NOT NULL,
    receiver_name TEXT NOT NULL DEFAULT '',
    package_name TEXT NOT NULL,
    start_line INTEGER NOT NULL CHECK (start_line > 0),
    end_line INTEGER NOT NULL CHECK (end_line >= start_line),
    change_type TEXT NOT NULL CHECK (change_type IN ('added', 'modified', 'deleted')),
    change_summary TEXT NOT NULL DEFAULT ''
);

CREATE INDEX changed_symbols_changed_file_id_idx ON changed_symbols (changed_file_id);
