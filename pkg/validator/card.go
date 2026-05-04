package validator

import "unicode"

// IsValidBIN checks that a BIN is between 6 and 8 digits
func IsValidBIN(bin string) bool {
	if len(bin) < 6 || len(bin) > 8 {
		return false
	}
	for _, r := range bin {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// Luhn validates a full card number using the Luhn algorithm
func Luhn(number string) bool {
	sum := 0
	parity := len(number) % 2
	for i, r := range number {
		if !unicode.IsDigit(r) {
			return false
		}
		digit := int(r - '0')
		if i%2 == parity {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}
	return sum%10 == 0
}
