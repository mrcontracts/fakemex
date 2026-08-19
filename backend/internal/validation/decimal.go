package validation

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalRE = regexp.MustCompile(`^(0|[1-9]\d*)(\.\d+)?$`)

const maxDecimalLength = 64

func ValidateDecimal(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("empty decimal")
	}
	if len(value) > maxDecimalLength {
		return fmt.Errorf("decimal is too long")
	}
	if !decimalRE.MatchString(value) {
		return fmt.Errorf("invalid decimal string")
	}
	if _, ok := new(big.Rat).SetString(value); !ok {
		return fmt.Errorf("invalid decimal numeric")
	}
	if value[0] == '-' {
		return fmt.Errorf("must be non-negative")
	}
	return nil
}

func ValidatePositiveDecimal(value string) error {
	if err := ValidateDecimal(value); err != nil {
		return err
	}
	parsed, _ := new(big.Rat).SetString(value)
	if parsed.Sign() <= 0 {
		return fmt.Errorf("must be greater than zero")
	}
	return nil
}

func ValidateScale(value string, maxDecimals int) error {
	if err := ValidateDecimal(value); err != nil {
		return err
	}
	if maxDecimals <= 0 {
		if strings.Contains(value, ".") {
			return fmt.Errorf("too many decimals (max %d)", maxDecimals)
		}
		return nil
	}
	value = strings.TrimSpace(value)
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 && len(parts[1]) > maxDecimals {
		return fmt.Errorf("too many decimals (max %d)", maxDecimals)
	}
	return nil
}
