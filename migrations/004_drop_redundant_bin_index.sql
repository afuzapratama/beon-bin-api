-- The UNIQUE constraint on bins.bin already owns an equivalent B-tree index.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'bins'::regclass
          AND contype = 'u'
          AND pg_get_constraintdef(oid) = 'UNIQUE (bin)'
    ) THEN
        RAISE EXCEPTION 'cannot remove idx_bins_bin without UNIQUE (bin) constraint';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_bins_bin;
