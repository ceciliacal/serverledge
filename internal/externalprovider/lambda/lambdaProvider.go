package lambda

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/serverledge-faas/serverledge/internal/externalprovider/lambda/utils"
	"github.com/serverledge-faas/serverledge/internal/function"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

var (
	// singleton instance
	providerInstance Provider
	once             sync.Once
	initErr          error
)

type Provider struct {
	client *lambda.Client
	role   string
}

const maxSyncPayloadBytes = 6 * 1024 * 1024
const awsHandlerName = "lambda_handler"
const RolePolicy = "arn:aws:iam::222255904815:role/lambda-simple-role"

func GetProvider() (Provider, error) {
	once.Do(func() {
		cfg, err := utils.LoadAWSConfig()
		if err != nil {
			initErr = fmt.Errorf("failed to load AWS config: %w", err)
			return
		}

		client := lambda.NewFromConfig(cfg)
		providerInstance = Provider{client: client}
	})

	if initErr != nil {
		return Provider{}, initErr
	}

	return providerInstance, nil
}

const defaultTimeout = 900

func (p Provider) CreateFunction(ctx context.Context, fn *function.Function) (string, error) {

	tarBytes, err := base64.StdEncoding.DecodeString(fn.TarFunctionCode)
	if err != nil {
		return "", fmt.Errorf("failed to decode tar: %w", err)
	}

	//Qui traduzione codice
	lambdaCode, err := transformServerledgeToAWSLambda(*fn, tarBytes)
	if err != nil {
		return "", fmt.Errorf("failed to convert code to a compatible AWS Lambda version: %w", err)
	}

	parts := strings.SplitN(fn.Handler, ".", 2)
	fileBase := parts[0] + ".py"

	zipBytes, err := createZipFromCode(lambdaCode, fileBase)
	if err != nil {
		return "", fmt.Errorf("failed to convert tar to zip: %w", err)
	}

	rt, err := getLambdaRuntimeName(fn.Runtime)
	if err != nil {
		return "", fmt.Errorf("runtime error: %w", err)
	}

	handler := parts[0] + "." + awsHandlerName

	input := &lambda.CreateFunctionInput{
		FunctionName: aws.String(fn.Name),
		Runtime:      rt,                     // es. "python3.10"
		Role:         aws.String(RolePolicy), // ARN del ruolo IAM
		Handler:      aws.String(handler),    // es. "hello.handler"
		Code: &types.FunctionCode{
			ZipFile: zipBytes,
		},
		MemorySize: aws.Int32(int32(fn.MemoryMB)), // es. 600
		Timeout:    aws.Int32(int32(defaultTimeout)),
		Publish:    true,
	}

	out, err := p.client.CreateFunction(ctx, input)
	if err != nil {
		log.Printf("error creating lambda function %q: %v", fn.Name, err)
		return "", err
	}

	log.Printf("Lambda function created: ARN=%s, State=%s, LastUpdateStatus=%s",
		aws.ToString(out.FunctionArn),
		out.State,
		out.LastUpdateStatus,
	)
	return aws.ToString(out.FunctionArn), nil
}

func createZipFromCode(pythonCode, filename string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Crea il file Python nel ZIP
	fileWriter, err := zipWriter.Create(filename) // es: "lambda_function.py"
	if err != nil {
		return nil, err
	}

	_, err = fileWriter.Write([]byte(pythonCode))
	if err != nil {
		return nil, err
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func getLambdaRuntimeName(runtime string) (types.Runtime, error) {
	switch runtime {
	case "python310", "python3.10":
		return types.RuntimePython310, nil
	default:
		return "", fmt.Errorf("unsupported runtime %q", runtime)
	}
}

// ListFunctions recupera solo i nomi delle Lambda, gestendo la paginazione
func (p Provider) ListFunctions(ctx context.Context) ([]string, error) {
	var names []string
	var marker *string

	for {
		input := &lambda.ListFunctionsInput{
			Marker:   marker,
			MaxItems: aws.Int32(100),
		}
		output, err := p.client.ListFunctions(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("error listing AWS Lambda functions: %w", err)
		}
		for _, fn := range output.Functions {
			names = append(names, aws.ToString(fn.FunctionName))
		}
		if output.NextMarker == nil {
			break
		}
		marker = output.NextMarker
	}

	return names, nil
}

func (p Provider) InvokeProviderFunction(request *function.Request, payload []byte) (function.ExecutionReport, error) {

	if request == nil || request.Fun.Name == "" {
		return function.ExecutionReport{}, fmt.Errorf("invalid request: missing function name")
	}

	if len(payload) > maxSyncPayloadBytes {
		return function.ExecutionReport{}, fmt.Errorf("payload too large: %d bytes (max %d)", len(payload), maxSyncPayloadBytes)
	}
	log.Printf("Richiamo la funzione con nome: %s\nValore ARN: %s\n", request.Fun.Name, request.Fun.ArnCode)

	in := &lambda.InvokeInput{
		FunctionName:   aws.String(request.Fun.ArnCode),
		Payload:        payload,
		InvocationType: types.InvocationTypeRequestResponse,
		LogType:        types.LogTypeTail, // ultimi 4KB di log (base64)
	}

	var report function.ExecutionReport

	out, err := p.client.Invoke(request.Ctx, in)
	if err != nil {
		// report rimane zero-value; il defer notificherà comunque
		return function.ExecutionReport{}, fmt.Errorf("invoke API failed: %w", err)
	}

	// gestione errori runtime Lambda
	if out.FunctionError != nil {
		return function.ExecutionReport{}, fmt.Errorf("lambda function error (%s): %s",
			aws.ToString(out.FunctionError), string(out.Payload))
	}

	// accetta 2xx
	if out.StatusCode < 200 || out.StatusCode > 299 {
		return function.ExecutionReport{}, fmt.Errorf("unexpected status code %d", out.StatusCode)
	}

	log.Printf("[AWS Lambda Invoke] Function=%s StatusCode=%d ExecutedVersion=%s PayloadLen=%dB",
		request.Fun.Name, out.StatusCode, aws.ToString(out.ExecutedVersion), len(out.Payload))

	// decodifica log tail se presente
	if out.LogResult != nil && len(*out.LogResult) > 0 {
		if tail, decErr := base64.StdEncoding.DecodeString(*out.LogResult); decErr == nil {
			log.Printf("[AWS Lambda Logs]\n%s", string(tail))
		} else {
			log.Printf("[AWS Lambda Logs] decode error: %v", decErr)
		}
	}

	var durSec float64
	if d, ok := utils.ExtractDurationFromLog(out.LogResult); ok {
		durSec = d
	}

	var initSec float64
	isWarm := true
	if v, ok := utils.ExtractInitDurationFromLog(out.LogResult); ok {
		initSec = v
		isWarm = false
	}

	report = function.ExecutionReport{
		Output:      string(out.Payload),
		IsWarmStart: isWarm,
		Duration:    durSec,  // da log
		InitTime:    initSec, // SOLO cold start da log
	}

	return report, nil
}

func (p Provider) DeleteProviderFunction(ctx context.Context, fn *function.Function) error {
	if fn == nil || (fn.Name == "") {
		return fmt.Errorf("delete: invalid request (missing name/ARN)")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var err error
	for attempt := 0; attempt < 3; attempt++ {
		_, err := p.client.DeleteFunction(ctx, &lambda.DeleteFunctionInput{
			FunctionName: aws.String(fn.ArnCode),
		})
		if err == nil {
			log.Printf("delete: function %q deleted", fn.Name)
			return nil
		}
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &notFound) {
			log.Printf("delete: function %q not found (idempotent)", fn.Name)
			return nil
		}
		var conflict *types.ResourceConflictException
		if errors.As(err, &conflict) {
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		return fmt.Errorf("delete %q failed: %w", fn.Name, err)
	}
	return fmt.Errorf("delete %q failed after retries: %w", fn.Name, err)
}
