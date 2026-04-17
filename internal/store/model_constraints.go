package store

import (
	"fmt"

	"gorm.io/gorm"
)

func enableSQLiteForeignKeys(db *gorm.DB) error {
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	return db.Exec("PRAGMA foreign_keys = ON").Error
}

func ensureModelConstraints(db *gorm.DB) error {
	if err := ensureModelRelationConstraint(db, &ModelMode{}, "Family"); err != nil {
		return err
	}
	if err := ensureModelRelationConstraint(db, &ModelFamily{}, "DefaultMode"); err != nil {
		return err
	}
	switch db.Dialector.Name() {
	case "postgres":
		return ensurePostgresModelTriggers(db)
	case "sqlite":
		return ensureSQLiteModelTriggers(db)
	default:
		return nil
	}
}

func ensureModelRelationConstraint(db *gorm.DB, model any, name string) error {
	if db.Migrator().HasConstraint(model, name) {
		return nil
	}
	if err := db.Migrator().CreateConstraint(model, name); err != nil {
		return fmt.Errorf("create %s constraint: %w", name, err)
	}
	return nil
}

func ensureSQLiteModelTriggers(db *gorm.DB) error {
	stmts := []string{
		`CREATE TRIGGER IF NOT EXISTS trg_model_families_default_mode_insert
		BEFORE INSERT ON model_families
		FOR EACH ROW
		WHEN NEW.default_mode_id IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM model_modes
		    WHERE id = NEW.default_mode_id AND model_id = NEW.id
		  )
		BEGIN
		  SELECT RAISE(ABORT, 'default_mode_id must belong to the same family');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_model_families_default_mode_update
		BEFORE UPDATE OF id, default_mode_id ON model_families
		FOR EACH ROW
		WHEN NEW.default_mode_id IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM model_modes
		    WHERE id = NEW.default_mode_id AND model_id = NEW.id
		  )
		BEGIN
		  SELECT RAISE(ABORT, 'default_mode_id must belong to the same family');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_model_modes_default_move
		BEFORE UPDATE OF model_id ON model_modes
		FOR EACH ROW
		WHEN EXISTS (
		  SELECT 1 FROM model_families
		  WHERE default_mode_id = OLD.id AND id <> NEW.model_id
		)
		BEGIN
		  SELECT RAISE(ABORT, 'default_mode_id must belong to the same family');
		END;`,
	}
	return execStatements(db, stmts)
}

func ensurePostgresModelTriggers(db *gorm.DB) error {
	stmts := []string{
		`CREATE OR REPLACE FUNCTION check_model_family_default_mode()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
		  IF NEW.default_mode_id IS NULL THEN
		    RETURN NEW;
		  END IF;
		  IF NOT EXISTS (
		    SELECT 1 FROM model_modes
		    WHERE id = NEW.default_mode_id AND model_id = NEW.id
		  ) THEN
		    RAISE EXCEPTION 'default_mode_id must belong to the same family';
		  END IF;
		  RETURN NEW;
		END;
		$$;`,
		`CREATE OR REPLACE FUNCTION check_model_mode_default_move()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
		  IF EXISTS (
		    SELECT 1 FROM model_families
		    WHERE default_mode_id = OLD.id AND id <> NEW.model_id
		  ) THEN
		    RAISE EXCEPTION 'default_mode_id must belong to the same family';
		  END IF;
		  RETURN NEW;
		END;
		$$;`,
		`DROP TRIGGER IF EXISTS trg_model_families_default_mode ON model_families;`,
		`CREATE TRIGGER trg_model_families_default_mode
		BEFORE INSERT OR UPDATE OF id, default_mode_id ON model_families
		FOR EACH ROW
		EXECUTE FUNCTION check_model_family_default_mode();`,
		`DROP TRIGGER IF EXISTS trg_model_modes_default_move ON model_modes;`,
		`CREATE TRIGGER trg_model_modes_default_move
		BEFORE UPDATE OF model_id ON model_modes
		FOR EACH ROW
		EXECUTE FUNCTION check_model_mode_default_move();`,
	}
	return execStatements(db, stmts)
}

func execStatements(db *gorm.DB, stmts []string) error {
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
