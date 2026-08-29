ALTER TABLE approvals ADD COLUMN tool_calls TEXT NOT NULL DEFAULT '[]';

ALTER TABLE run_tool_checkpoints ADD COLUMN approved_calls TEXT NOT NULL DEFAULT '[]';
ALTER TABLE run_tool_checkpoints ADD COLUMN denied_calls TEXT NOT NULL DEFAULT '[]';
