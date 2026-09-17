DROP TABLE IF EXISTS document_set_purge_audit;
DROP TABLE IF EXISTS document_ai_budget_reservations;

DELETE FROM document_set_audit_log WHERE action = 'BUDGET_CHANGED';
ALTER TABLE document_set_audit_log
    DROP CONSTRAINT document_set_audit_log_action_check,
    ADD CONSTRAINT document_set_audit_log_action_check CHECK
        (action IN ('ARCHIVED', 'RESTORED', 'RETENTION_CHANGED'));

UPDATE document_sets SET status = 'ARCHIVED' WHERE status = 'PURGING';
ALTER TABLE document_sets
    DROP CONSTRAINT document_sets_status_check,
    ADD CONSTRAINT document_sets_status_check CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    DROP COLUMN ai_cost_budget_microusd,
    DROP COLUMN ai_token_budget;
