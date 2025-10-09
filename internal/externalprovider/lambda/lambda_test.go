package lambda

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/serverledge-faas/serverledge/internal/function"
	"strings"
	"sync/atomic"
	"testing"
)

// mockLambdaClient è un client finto che implementa l'interfaccia LambdaAPI.
type mockLambdaClient struct {
	InvokeFunc         func(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
	CreateFunctionFunc func(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error)
	DeleteFunctionFunc func(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error)
	ListFunctionsFunc  func(ctx context.Context, params *lambda.ListFunctionsInput, optFns ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
}

// Implementiamo i metodi dell'interfaccia.
func (m *mockLambdaClient) Invoke(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
	if m.InvokeFunc != nil {
		return m.InvokeFunc(ctx, params)
	}
	return &lambda.InvokeOutput{}, nil
}
func (m *mockLambdaClient) CreateFunction(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error) {
	if m.CreateFunctionFunc != nil {
		return m.CreateFunctionFunc(ctx, params)
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:" + *params.FunctionName
	return &lambda.CreateFunctionOutput{FunctionArn: aws.String(arn)}, nil
}
func (m *mockLambdaClient) DeleteFunction(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
	if m.DeleteFunctionFunc != nil {
		return m.DeleteFunctionFunc(ctx, params)
	}
	return &lambda.DeleteFunctionOutput{}, nil
}
func (m *mockLambdaClient) ListFunctions(ctx context.Context, params *lambda.ListFunctionsInput, optFns ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	if m.ListFunctionsFunc != nil {
		return m.ListFunctionsFunc(ctx, params)
	}
	return &lambda.ListFunctionsOutput{}, nil
}

// TestGetLambdaRuntimeName verifica la corretta mappatura dei nomi dei runtime.
func TestGetLambdaRuntimeName(t *testing.T) {
	testCases := []struct {
		name      string
		runtime   string
		want      types.Runtime
		expectErr bool
	}{
		{"Standard Python 3.10", "python310", types.RuntimePython310, false},
		{"Python with dot", "python3.10", types.RuntimePython310, false},
		{"Unsupported Runtime", "go1.x", "", true},
		{"Empty Runtime", "", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getLambdaRuntimeName(tc.runtime)
			if (err != nil) != tc.expectErr {
				t.Fatalf("getLambdaRuntimeName() error = %v, wantErr %v", err, tc.expectErr)
			}
			if got != tc.want {
				t.Errorf("getLambdaRuntimeName() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCreateZipFromCode verifica che la zippatura del codice funzioni correttamente.
func TestCreateZipFromCode(t *testing.T) {
	code := "def lambda_handler(event, context): return 'hello'"
	filename := "main.py"

	zipBytes, err := CreateZipFromCode(code, filename)
	if err != nil {
		t.Fatalf("CreateZipFromCode() failed: %v", err)
	}

	// Verifica che il risultato sia un archivio zip valido
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("Failed to read zip archive: %v", err)
	}

	if len(zipReader.File) != 1 {
		t.Fatalf("Expected 1 file in zip, got %d", len(zipReader.File))
	}

	fileInZip := zipReader.File[0]
	if fileInZip.Name != filename {
		t.Errorf("Expected filename to be %s, got %s", filename, fileInZip.Name)
	}
}

// TestInvokeProviderFunction verifica la logica di invocazione e parsing dei log.
func TestInvokeProviderFunction(t *testing.T) {
	mockClient := &mockLambdaClient{}
	provider := Provider{client: mockClient}
	testFunc := &function.Function{Name: "test-func", ArnCode: "arn:test"}

	t.Run("Warm Start", func(t *testing.T) {
		// Simula un log di una esecuzione "warm"
		logResult := "START RequestId: ... Version: $LATEST\nEND RequestId: ...\nREPORT RequestId: ...\tDuration: 12.34 ms\tBilled Duration: ...\n"
		mockClient.InvokeFunc = func(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				Payload:    []byte(`{"result": "ok"}`),
				LogResult:  aws.String(base64.StdEncoding.EncodeToString([]byte(logResult))),
				StatusCode: 200,
			}, nil
		}

		report, err := provider.InvokeProviderFunction(&function.Request{Fun: testFunc, Ctx: context.Background()}, []byte{})
		if err != nil {
			t.Fatalf("Invoke failed unexpectedly: %v", err)
		}

		if !report.IsWarmStart {
			t.Error("Expected IsWarmStart to be true")
		}
		if report.InitTime != 0 {
			t.Errorf("Expected InitTime to be 0 for warm start, got %f", report.InitTime)
		}
		if report.Result != `{"result": "ok"}` {
			t.Errorf("Unexpected result payload: got %s", report.Result)
		}
	})

	t.Run("Cold Start", func(t *testing.T) {
		// Simula un log di una esecuzione "cold"
		logResult := "START RequestId: ...\nEND RequestId: ...\nREPORT RequestId: ...\tDuration: 15.67 ms\tInit Duration: 123.45 ms\tBilled Duration: ...\n"
		mockClient.InvokeFunc = func(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				Payload:    []byte(`{"result": "ok"}`),
				LogResult:  aws.String(base64.StdEncoding.EncodeToString([]byte(logResult))),
				StatusCode: 200,
			}, nil
		}

		report, err := provider.InvokeProviderFunction(&function.Request{Fun: testFunc, Ctx: context.Background()}, []byte{})
		if err != nil {
			t.Fatalf("Invoke failed unexpectedly: %v", err)
		}

		if report.IsWarmStart {
			t.Error("Expected IsWarmStart to be false")
		}
		// Controlla se il tempo di init è stato parsato correttamente (123.45ms -> 0.12345s)
		if report.InitTime < 0.123 || report.InitTime > 0.124 {
			t.Errorf("Expected InitTime to be around 0.12345, got %f", report.InitTime)
		}
	})

	t.Run("Function Error", func(t *testing.T) {
		// Simula un errore di runtime nella funzione Lambda
		mockClient.InvokeFunc = func(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
			return &lambda.InvokeOutput{
				FunctionError: aws.String("Runtime.ImportModuleError"),
				Payload:       []byte(`{"errorMessage": "Unable to import module 'app'"}`),
				StatusCode:    200, // Anche con errori di funzione, lo status code è 200
			}, nil
		}

		_, err := provider.InvokeProviderFunction(&function.Request{Fun: testFunc, Ctx: context.Background()}, []byte{})
		if err == nil {
			t.Fatal("Expected an error for FunctionError, but got nil")
		}
		if !strings.Contains(err.Error(), "lambda function error") {
			t.Errorf("Expected error to contain 'lambda function error', got: %v", err)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		// Simula un errore dell'API di AWS (es. credenziali sbagliate)
		mockClient.InvokeFunc = func(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
			return nil, errors.New("InvalidParameterValueException")
		}

		_, err := provider.InvokeProviderFunction(&function.Request{Fun: testFunc, Ctx: context.Background()}, []byte{})
		if err == nil {
			t.Fatal("Expected an API error, but got nil")
		}
		if !strings.Contains(err.Error(), "invoke API failed") {
			t.Errorf("Expected error to contain 'invoke API failed', got: %v", err)
		}
	})
}

// TestCreateFunction verifica la logica di creazione di una funzione Lambda.
func TestCreateFunction(t *testing.T) {
	mockClient := &mockLambdaClient{}
	testFunc := &function.Function{
		Name:            "my-test-func",
		MemoryMB:        256,
		Runtime:         "python310",
		Handler:         "main.handler",
		TarFunctionCode: base64.StdEncoding.EncodeToString([]byte("dummy-tar-data")),
	}

	t.Run("Success Case", func(t *testing.T) {
		provider := Provider{
			client: mockClient,
			transformer: func(fn function.Function, code []byte) (string, error) {
				return "transformed_code_for_test", nil
			},
		}
		var capturedInput *lambda.CreateFunctionInput
		mockClient.CreateFunctionFunc = func(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error) {
			capturedInput = params
			return &lambda.CreateFunctionOutput{FunctionArn: aws.String("arn:success")}, nil
		}

		arn, err := provider.CreateFunction(context.Background(), testFunc)
		if err != nil {
			t.Fatalf("CreateFunction failed unexpectedly: %v", err)
		}
		if arn != "arn:success" {
			t.Errorf("Expected ARN 'arn:success', got '%s'", arn)
		}

		if capturedInput == nil {
			t.Fatal("CreateFunction was not called on the mock client")
		}
		if aws.ToString(capturedInput.FunctionName) != "my-test-func" {
			t.Errorf("Expected FunctionName 'my-test-func', got '%s'", aws.ToString(capturedInput.FunctionName))
		}
	})

	t.Run("Transformer Error", func(t *testing.T) {
		// Simula un errore nella fase di trasformazione del codice
		provider := Provider{
			client: mockClient,
			transformer: func(fn function.Function, code []byte) (string, error) {
				return "", errors.New("transformation failed")
			},
		}
		_, err := provider.CreateFunction(context.Background(), testFunc)
		if err == nil {
			t.Fatal("Expected an error from transformer, but got nil")
		}
		if !strings.Contains(err.Error(), "transformation failed") {
			t.Errorf("Error message mismatch: %v", err)
		}
	})

	t.Run("API Error", func(t *testing.T) {
		provider := Provider{
			client: mockClient,
			transformer: func(fn function.Function, code []byte) (string, error) {
				return "code", nil
			},
		}
		mockClient.CreateFunctionFunc = func(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error) {
			return nil, errors.New("AWS API error")
		}
		_, err := provider.CreateFunction(context.Background(), testFunc)
		if err == nil {
			t.Fatal("Expected an error from API, but got nil")
		}
	})
}

// TestListFunctions verifica la logica di paginazione, fondamentale
// quando si hanno molte funzioni su AWS.
func TestListFunctions(t *testing.T) {
	mockClient := &mockLambdaClient{}
	provider := Provider{client: mockClient}

	t.Run("Pagination Logic", func(t *testing.T) {
		// Simula una risposta paginata da AWS
		mockClient.ListFunctionsFunc = func(ctx context.Context, params *lambda.ListFunctionsInput, optFns ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
			if params.Marker == nil {
				// Prima pagina: restituisce una funzione e un marcatore per la pagina successiva
				return &lambda.ListFunctionsOutput{
					Functions:  []types.FunctionConfiguration{{FunctionName: aws.String("func-1")}},
					NextMarker: aws.String("page-2-marker"),
				}, nil
			} else if aws.ToString(params.Marker) == "page-2-marker" {
				// Seconda pagina: restituisce un'altra funzione e nessun marcatore (fine)
				return &lambda.ListFunctionsOutput{
					Functions:  []types.FunctionConfiguration{{FunctionName: aws.String("func-2")}},
					NextMarker: nil,
				}, nil
			}
			return nil, fmt.Errorf("marcatore di paginazione inaspettato: %s", aws.ToString(params.Marker))
		}

		names, err := provider.ListFunctions(context.Background())
		if err != nil {
			t.Fatalf("ListFunctions failed unexpectedly: %v", err)
		}

		if len(names) != 2 {
			t.Fatalf("Expected 2 function names, got %d", len(names))
		}
		if names[0] != "func-1" || names[1] != "func-2" {
			t.Errorf("Expected names ['func-1', 'func-2'], got %v", names)
		}
	})
}

// TestDeleteProviderFunction verifica la robustezza della logica di cancellazione.
func TestDeleteProviderFunction(t *testing.T) {
	mockClient := &mockLambdaClient{}
	provider := Provider{client: mockClient}
	testFunc := &function.Function{Name: "test-func", ArnCode: "arn:test"}

	t.Run("Success Case", func(t *testing.T) {
		// Simula una cancellazione avvenuta con successo
		var called bool
		mockClient.DeleteFunctionFunc = func(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
			called = true
			return &lambda.DeleteFunctionOutput{}, nil
		}
		err := provider.DeleteProviderFunction(context.Background(), testFunc)
		if err != nil {
			t.Fatalf("DeleteProviderFunction failed unexpectedly: %v", err)
		}
		if !called {
			t.Error("DeleteFunction API was not called")
		}
	})

	t.Run("Not Found is Idempotent", func(t *testing.T) {
		// Simula il caso in cui la funzione sia già stata cancellata.
		// L'operazione deve avere successo (idempotenza).
		mockClient.DeleteFunctionFunc = func(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
			return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
		}
		err := provider.DeleteProviderFunction(context.Background(), testFunc)
		if err != nil {
			t.Fatalf("Expected nil error for ResourceNotFound, got %v", err)
		}
	})

	t.Run("Resource Conflict with Retry", func(t *testing.T) {
		// Simula un conflitto temporaneo, verificando che la logica di retry funzioni.
		var callCount atomic.Int32
		mockClient.DeleteFunctionFunc = func(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
			count := callCount.Add(1)
			if count == 1 { // Al primo tentativo, restituisce un errore di conflitto
				return nil, &types.ResourceConflictException{Message: aws.String("in use")}
			}
			// Al secondo tentativo, ha successo
			return &lambda.DeleteFunctionOutput{}, nil
		}
		err := provider.DeleteProviderFunction(context.Background(), testFunc)
		if err != nil {
			t.Fatalf("Expected success after retry, but got error: %v", err)
		}
		if callCount.Load() < 2 {
			t.Errorf("Expected DeleteFunction to be called more than once for retry, but was called %d times", callCount.Load())
		}
	})
}

// TestInvokeProviderFunction_InputValidation copre i controlli di validazione
// all'inizio della funzione di invocazione.
func TestInvokeProviderFunction_InputValidation(t *testing.T) {
	provider := Provider{client: &mockLambdaClient{}}

	t.Run("Nil Request", func(t *testing.T) {
		_, err := provider.InvokeProviderFunction(nil, []byte{})
		if err == nil {
			t.Fatal("Expected an error for nil request, but got nil")
		}
	})

	t.Run("Payload Too Large", func(t *testing.T) {
		largePayload := make([]byte, maxSyncPayloadBytes+1)
		testFunc := &function.Function{Name: "test-func", ArnCode: "arn:test"}
		_, err := provider.InvokeProviderFunction(&function.Request{Fun: testFunc, Ctx: context.Background()}, largePayload)
		if err == nil {
			t.Fatal("Expected an error for large payload, but got nil")
		}
		if !strings.Contains(err.Error(), "payload too large") {
			t.Errorf("Expected error to contain 'payload too large', got: %v", err)
		}
	})
}
