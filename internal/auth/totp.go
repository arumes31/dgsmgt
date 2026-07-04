package auth

import (
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret creates a new TOTP secret for a user and returns the
// secret plus the otpauth:// provisioning URL for QR codes.
func GenerateTOTPSecret(username string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "DGSMgt",
		AccountName: username,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTP checks a 6-digit code against a secret.
var ValidateTOTP = func(code, secret string) bool {
	return totp.Validate(code, secret)
}
