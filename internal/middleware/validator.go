package middleware

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v2"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Validator struct {
	errors []ValidationError
}

func NewValidator() *Validator {
	return &Validator{
		errors: make([]ValidationError, 0),
	}
}

func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

func (v *Validator) Errors() []ValidationError {
	return v.errors
}

func (v *Validator) AddError(field, message string) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.AddError(field, field+" is required")
	}
	return v
}

func (v *Validator) Email(field, value string) *Validator {
	if value == "" {
		return v
	}
	_, err := mail.ParseAddress(value)
	if err != nil {
		v.AddError(field, "Invalid email format")
	}
	return v
}

func (v *Validator) MinLength(field, value string, min int) *Validator {
	if value == "" {
		return v
	}
	if len(value) < min {
		v.AddError(field, field+" must be at least "+string(rune(min+'0'))+" characters")
	}
	return v
}

func (v *Validator) MaxLength(field, value string, max int) *Validator {
	if value == "" {
		return v
	}
	if len(value) > max {
		v.AddError(field, field+" must be at most "+string(rune(max+'0'))+" characters")
	}
	return v
}

func (v *Validator) Password(field, value string) *Validator {
	if value == "" {
		return v
	}

	var (
		hasMinLen  = len(value) >= 8
		hasUpper   = false
		hasLower   = false
		hasNumber  = false
		hasSpecial = false
	)

	for _, char := range value {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasMinLen {
		v.AddError(field, "Password must be at least 8 characters")
	}
	if !hasUpper {
		v.AddError(field, "Password must contain at least one uppercase letter")
	}
	if !hasLower {
		v.AddError(field, "Password must contain at least one lowercase letter")
	}
	if !hasNumber {
		v.AddError(field, "Password must contain at least one number")
	}
	if !hasSpecial {
		v.AddError(field, "Password must contain at least one special character")
	}

	return v
}

func (v *Validator) AlphaNumeric(field, value string) *Validator {
	if value == "" {
		return v
	}
	matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", value)
	if !matched {
		v.AddError(field, field+" must contain only letters and numbers")
	}
	return v
}

func (v *Validator) NoSQLInjection(field, value string) *Validator {
	if value == "" {
		return v
	}

	dangerousPatterns := []string{
		"--",
		";--",
		"/*",
		"*/",
		"@@",
		"char(",
		"nchar(",
		"varchar(",
		"nvarchar(",
		"alter ",
		"begin ",
		"cast(",
		"create ",
		"cursor ",
		"declare ",
		"delete ",
		"drop ",
		"end ",
		"exec(",
		"execute(",
		"fetch ",
		"insert ",
		"kill ",
		"select ",
		"sys.",
		"sysobjects",
		"syscolumns",
		"table ",
		"update ",
		"union ",
		"' or ",
		"'or ",
		"' and ",
		"'and ",
		"1=1",
		"1 = 1",
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerValue, pattern) {
			v.AddError(field, "Invalid characters detected")
			return v
		}
	}
	return v
}

func (v *Validator) NoXSS(field, value string) *Validator {
	if value == "" {
		return v
	}

	dangerousPatterns := []string{
		"<script",
		"</script>",
		"javascript:",
		"onerror=",
		"onload=",
		"onclick=",
		"onmouseover=",
		"onfocus=",
		"onblur=",
		"<iframe",
		"<object",
		"<embed",
		"<svg",
		"<img",
		"expression(",
		"vbscript:",
		"data:",
	}

	lowerValue := strings.ToLower(value)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerValue, pattern) {
			v.AddError(field, "Invalid content detected")
			return v
		}
	}
	return v
}

func ValidateBody(validateFunc func(c *fiber.Ctx, v *Validator) error) fiber.Handler {
	return func(c *fiber.Ctx) error {
		v := NewValidator()

		if err := validateFunc(c, v); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": err.Error(),
			})
		}

		if v.HasErrors() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation Failed",
				"details": v.Errors(),
			})
		}

		return c.Next()
	}
}

func SanitizeInput(input string) string {
	input = strings.ReplaceAll(input, "\x00", "")
	input = strings.TrimSpace(input)

	replacer := strings.NewReplacer(
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
		"&", "&amp;",
	)

	return replacer.Replace(input)
}
