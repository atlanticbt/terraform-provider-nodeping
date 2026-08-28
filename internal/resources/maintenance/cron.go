package maintenance

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// cronField describes the permitted values for one position of a standard
// 5-field cron line.
type cronField struct {
	name   string
	min    int
	max    int
	names  map[string]int
	allowQ bool // '?' is accepted in the day-of-month and day-of-week positions
}

var cronFields = []cronField{
	{name: "minute", min: 0, max: 59},
	{name: "hour", min: 0, max: 23},
	{name: "day of month", min: 1, max: 31, allowQ: true},
	{name: "month", min: 1, max: 12, names: monthNames},
	{name: "day of week", min: 0, max: 7, names: dayNames, allowQ: true},
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var dayNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// ValidateCronExpression checks that s is a well-formed 5-field cron line.
// It returns a human-readable reason when the expression is rejected.
//
// Accepted per field: `*`, `?` (day positions only), a literal value, a `a-b`
// range, a comma-separated list of those, and a `/n` step suffix on `*` or on a
// range. Month and day-of-week fields additionally accept three-letter names.
func ValidateCronExpression(s string) error {
	if strings.ContainsAny(s, "\x00\r\n\t") {
		return fmt.Errorf("must not contain control characters")
	}

	fields := strings.Fields(s)
	if len(fields) != 5 {
		return fmt.Errorf("must have exactly 5 whitespace-separated fields (minute hour day-of-month month day-of-week), got %d", len(fields))
	}

	for i, raw := range fields {
		if err := validateCronField(cronFields[i], raw); err != nil {
			return fmt.Errorf("%s field %q is invalid: %w", cronFields[i].name, raw, err)
		}
	}

	return nil
}

func validateCronField(f cronField, raw string) error {
	if raw == "?" {
		if !f.allowQ {
			return fmt.Errorf("'?' is only allowed in the day-of-month and day-of-week fields")
		}
		return nil
	}

	for _, item := range strings.Split(raw, ",") {
		if item == "" {
			return fmt.Errorf("empty list entry")
		}
		if err := validateCronItem(f, item); err != nil {
			return err
		}
	}

	return nil
}

func validateCronItem(f cronField, item string) error {
	base := item
	if slash := strings.Index(item, "/"); slash >= 0 {
		base = item[:slash]
		step := item[slash+1:]
		n, err := strconv.Atoi(step)
		if err != nil {
			return fmt.Errorf("step %q is not a number", step)
		}
		if n < 1 || n > f.max {
			return fmt.Errorf("step %d is out of range 1-%d", n, f.max)
		}
		if base == "" {
			return fmt.Errorf("step is missing a base value")
		}
	}

	if base == "*" {
		return nil
	}

	if dash := strings.Index(base, "-"); dash >= 0 {
		lo, err := cronValue(f, base[:dash])
		if err != nil {
			return err
		}
		hi, err := cronValue(f, base[dash+1:])
		if err != nil {
			return err
		}
		if lo > hi {
			return fmt.Errorf("range start %d is greater than range end %d", lo, hi)
		}
		return nil
	}

	_, err := cronValue(f, base)
	return err
}

func cronValue(f cronField, s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}

	if f.names != nil {
		if v, ok := f.names[strings.ToLower(s)]; ok {
			return v, nil
		}
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number in range %d-%d", s, f.min, f.max)
	}
	if v < f.min || v > f.max {
		return 0, fmt.Errorf("%d is out of range %d-%d", v, f.min, f.max)
	}
	return v, nil
}

// cronValidator rejects configuration values that are not a 5-field cron line.
type cronValidator struct{}

// CronValidator returns the schema validator for the `cron` attribute.
func CronValidator() validator.String {
	return cronValidator{}
}

func (v cronValidator) Description(_ context.Context) string {
	return "must be a standard 5-field cron expression (minute hour day-of-month month day-of-week)"
}

func (v cronValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v cronValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if err := ValidateCronExpression(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Cron Expression",
			fmt.Sprintf("Attribute %s %s. Value %q: %v.",
				req.Path, v.Description(ctx), req.ConfigValue.ValueString(), err),
		)
	}
}
