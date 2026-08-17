-- Savings metadata for recommendations (ranges, classification, overlap).
ALTER TABLE recommendations ADD COLUMN savings_low_minor INTEGER;
ALTER TABLE recommendations ADD COLUMN savings_high_minor INTEGER;
ALTER TABLE recommendations ADD COLUMN savings_class TEXT;
ALTER TABLE recommendations ADD COLUMN investigation_only INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recommendations ADD COLUMN overlap_key TEXT;
ALTER TABLE recommendations ADD COLUMN savings_inputs_json TEXT;
