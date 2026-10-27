// Package enrichment applies client-supplied updates (PATCH) to persons and
// companies: validation helpers, the merge itself, field provenance and the
// audit trail. It is shared so both entities follow exactly the same rules.
package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"atm-identity-api/internal/platform/apierror"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Target describes one enrichable table. Table and column names come from
// constants in the calling package, never from user input.
type Target struct {
	Table      string
	EntityType string            // "person" | "company"
	Columns    map[string]string // column -> SQL cast ("" when none)
}

// ErrNotFound means there is no row for the document number.
var ErrNotFound = errors.New("enrichment: record not found")

// Apply writes the given non-empty values, only where they differ from what is
// stored, inside one transaction. It records the change in record_changes and
// the writing system in field_sources. Fields not in updates are never touched.
func Apply(ctx context.Context, pool *pgxpool.Pool, t Target, documentNumber, clientCode string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	var raw []byte
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT id, to_jsonb(t) FROM %s t WHERE document_number = $1 FOR UPDATE`, t.Table),
		documentNumber).Scan(&id, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var current map[string]any
	if err := json.Unmarshal(raw, &current); err != nil {
		return err
	}

	changes := map[string]map[string]any{}
	sources := map[string]string{}
	sets := []string{}
	args := []any{id}
	for column, value := range updates {
		cast, ok := t.Columns[column]
		if !ok {
			return fmt.Errorf("enrichment: column %q is not enrichable", column)
		}
		old, _ := current[column].(string)
		if old == value {
			continue
		}
		args = append(args, value)
		expr := fmt.Sprintf("$%d", len(args))
		if cast != "" {
			expr += "::" + cast
		}
		sets = append(sets, column+" = "+expr)
		changes[column] = map[string]any{"old": nilIfEmpty(old), "new": value}
		sources[column] = "system:" + clientCode
	}
	if len(sets) == 0 {
		return nil
	}

	sourcesJSON, _ := json.Marshal(sources)
	args = append(args, string(sourcesJSON))
	sets = append(sets, fmt.Sprintf("field_sources = field_sources || $%d::jsonb", len(args)), "updated_at = now()")
	if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s WHERE id = $1`, t.Table, strings.Join(sets, ", ")), args...); err != nil {
		return err
	}

	changesJSON, _ := json.Marshal(changes)
	if _, err := tx.Exec(ctx, `
		INSERT INTO record_changes (entity_type, entity_id, client_system_id, changes)
		VALUES ($1, $2, (SELECT id FROM client_systems WHERE code = $3), $4::jsonb)`,
		t.EntityType, id, clientCode, string(changesJSON)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---- validation helpers: each returns an issue message, or "" when valid ----

var (
	phonePattern  = regexp.MustCompile(`^\+?[0-9][0-9 \-]{5,19}$`)
	ubigeoPattern = regexp.MustCompile(`^[0-9]{6}$`)
)

// Text checks an optional free-text field (trimmed, non-empty, bounded).
func Text(s string, max int) string {
	switch {
	case strings.TrimSpace(s) == "":
		return "must not be empty; omit the field instead"
	case len(s) > max:
		return fmt.Sprintf("must be at most %d characters", max)
	}
	return ""
}

func Email(s string) string {
	if len(s) > 254 {
		return "must be at most 254 characters"
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return "must be a valid email address"
	}
	return ""
}

func Phone(s string) string {
	if !phonePattern.MatchString(s) {
		return "must be 6 to 20 digits, optionally starting with + and using spaces or dashes"
	}
	return ""
}

func Ubigeo(s string) string {
	if !ubigeoPattern.MatchString(s) {
		return "must have exactly 6 digits"
	}
	return ""
}

func BirthDate(s string) string {
	d, err := time.Parse("2006-01-02", s)
	switch {
	case err != nil:
		return "must be a date in YYYY-MM-DD format"
	case d.After(time.Now()):
		return "must not be in the future"
	case d.Year() < 1900:
		return "must be 1900 or later"
	}
	return ""
}

func Sex(s string) string {
	if s != "male" && s != "female" {
		return `must be "male" or "female"`
	}
	return ""
}

// Collect builds the details list from field -> issue pairs, skipping valid ones.
func Collect(checks ...Check) []apierror.Detail {
	var out []apierror.Detail
	for _, c := range checks {
		if c.Value != nil && c.Issue != "" {
			out = append(out, apierror.Detail{Field: c.Field, Issue: c.Issue})
		}
	}
	return out
}

// Check is one field validation: Value nil means the field was not sent.
type Check struct {
	Field string
	Value *string
	Issue string
}

// Run validates value with fn when it was sent.
func Run(field string, value *string, fn func(string) string) Check {
	c := Check{Field: field, Value: value}
	if value != nil {
		c.Issue = fn(*value)
	}
	return c
}
