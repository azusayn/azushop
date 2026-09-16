-- Add headers column to order_outbox_messages if it doesn't exist.
-- This handles existing DBs where the schema was created before 001_init.sql was updated.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'order_outbox_messages' AND column_name = 'headers'
  ) THEN
    ALTER TABLE order_outbox_messages ADD COLUMN headers JSONB;
  END IF;
END
$$;
