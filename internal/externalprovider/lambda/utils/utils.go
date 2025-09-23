package utils

import (
	"context"
	"encoding/base64"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"regexp"
	"strconv"
)

const CredentialsDirectory string = "./internal/externalprovider/lambda/aws/credentials"
const ConfigDirectory string = "./internal/externalprovider/lambda/aws/config"
const DefaultProfile = "lambda"
const ServerUrlLambda = "aws:externalprovider"

func LoadAWSConfig() (aws.Config, error) {
	return config.LoadDefaultConfig(context.TODO(),
		config.WithSharedCredentialsFiles([]string{CredentialsDirectory}),
		config.WithSharedConfigFiles([]string{ConfigDirectory}),
		config.WithSharedConfigProfile(DefaultProfile),
	)
}

// ExtractDurationFromLog prende LogResult (base64) ed estrae solo la Duration in secondi.
// Restituisce (durata, ok).
func ExtractDurationFromLog(logResultB64 *string) (float64, bool) {
	if logResultB64 == nil || len(*logResultB64) == 0 {
		return 0, false
	}
	raw, err := base64.StdEncoding.DecodeString(*logResultB64)
	if err != nil {
		return 0, false
	}
	// Esempio riga: "REPORT RequestId: ... Duration: 1.57 ms ..."
	re := regexp.MustCompile(`Duration:\s*([\d\.]+)\s*ms`)
	m := re.FindSubmatch(raw)
	if m == nil {
		return 0, false
	}
	ms, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, false
	}
	return ms / 1000.0, true
}

// ExtractInitDurationFromLog prende LogResult (base64) e prova ad estrarre l'Init Duration in secondi.
// Restituisce (durata, ok).
func ExtractInitDurationFromLog(logResultB64 *string) (float64, bool) {
	if logResultB64 == nil || len(*logResultB64) == 0 {
		return 0.0, false
	}

	raw, err := base64.StdEncoding.DecodeString(*logResultB64)
	if err != nil {
		return 0.0, false
	}

	// Esempio riga: "REPORT RequestId: ... Init Duration: 105.08 ms ..."
	re := regexp.MustCompile(`Init Duration:\s*([\d\.]+)\s*ms`)
	m := re.FindSubmatch(raw)
	if m == nil {
		return 0.0, false
	}

	ms, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0.0, false
	}

	return ms / 1000.0, true
}
