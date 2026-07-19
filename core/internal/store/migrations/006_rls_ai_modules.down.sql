DROP POLICY IF EXISTS memory_turns_isolation ON memory_turns;
DROP POLICY IF EXISTS prompts_global ON prompts;
DROP POLICY IF EXISTS dlq_admin_only ON dlq;

ALTER TABLE memory_turns  DISABLE ROW LEVEL SECURITY;
ALTER TABLE prompts       DISABLE ROW LEVEL SECURITY;
ALTER TABLE dlq           DISABLE ROW LEVEL SECURITY;
