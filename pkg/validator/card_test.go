package validator

import "testing"

func TestIsValidBIN(t *testing.T) {
	tests := []struct {
		name string
		bin  string
		want bool
	}{
		{name: "six digits", bin: "411111", want: true},
		{name: "eight digits", bin: "45717360", want: true},
		{name: "seven digits", bin: "4111111", want: false},
		{name: "too short", bin: "41111", want: false},
		{name: "too long", bin: "411111111", want: false},
		{name: "ASCII letters", bin: "41111A", want: false},
		{name: "Arabic-Indic digits", bin: "١٢٣٤٥٦", want: false},
		{name: "spaces", bin: "411 111", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsValidBIN(test.bin); got != test.want {
				t.Fatalf("IsValidBIN(%q) = %v, want %v", test.bin, got, test.want)
			}
		})
	}
}

func TestLuhnIsOnlyAChecksum(t *testing.T) {
	if !Luhn("4111111111111111") {
		t.Fatal("known checksum-valid test number was rejected")
	}
	if Luhn("4111111111111112") {
		t.Fatal("checksum-invalid number was accepted")
	}
	if Luhn("４１１１１１１１１１１１１１１") {
		t.Fatal("non-ASCII digits were accepted")
	}
	if Luhn("") || Luhn("0") {
		t.Fatal("empty or too-short number was accepted")
	}
}
