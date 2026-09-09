package validator

// IsValidBIN checks that a BIN is exactly 6 or 8 ASCII digits.
func IsValidBIN(bin string) bool {
	if len(bin) != 6 && len(bin) != 8 {
		return false
	}
	for i := range len(bin) {
		if bin[i] < '0' || bin[i] > '9' {
			return false
		}
	}
	return true
}

// Luhn checks number syntax and checksum only. A passing result does not prove
// that a card exists, is active, or can be used for a transaction.
func Luhn(number string) bool {
	if len(number) < 13 || len(number) > 19 {
		return false
	}
	sum := 0
	parity := len(number) % 2
	for i := range len(number) {
		if number[i] < '0' || number[i] > '9' {
			return false
		}
		digit := int(number[i] - '0')
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
