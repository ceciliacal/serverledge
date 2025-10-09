package lambda

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/serverledge-faas/serverledge/internal/externalprovider/lambda/utils"
	"log"
	"sort"
	"sync"
	"time"
)

type RTTBatchOpts struct {
	Attempts  int     // Total number of invocation
	Warmup    int     // How many discard at beginning
	TrimRatio float64 // % to cut in up/down
	Timeout   time.Duration
}

type RttMonitor struct {
	latestRtt    time.Duration
	mutex        sync.RWMutex // Mutex per proteggere l'accesso a latestRtt
	lambdaFnName string
}

// Global instance
var defaultRttMonitor *RttMonitor

func InitRttMonitor(updateInterval time.Duration, fnArn string) {
	if defaultRttMonitor != nil {
		log.Println("RTT Monitor is up")
		return
	}

	log.Printf("Starting RTT Monitor. Update every %v seconds.", updateInterval)
	monitor := &RttMonitor{
		lambdaFnName: fnArn,
		latestRtt:    100 * time.Millisecond, // Un valore di default ragionevole
	}
	defaultRttMonitor = monitor

	// Avvia la goroutine che eseguirà il polling in background.
	go monitor.monitorLoop(updateInterval)
}

func (p Provider) GetRtt() time.Duration {
	if defaultRttMonitor == nil {
		log.Printf("Warning: RTT Monitor not initialized, sending default value...")
		return 40 * time.Millisecond //Approximation for a Europe Region like Frankfurt
	}
	return defaultRttMonitor.get()
}

func (m *RttMonitor) get() time.Duration {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.latestRtt
}

func (m *RttMonitor) monitorLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Println("First misuration...")
	m.updateRtt()

	for range ticker.C {
		log.Println("Polling RTT: new misuration...")
		m.updateRtt()
	}
}

func (m *RttMonitor) updateRtt() {
	newRtt, err := m.measure()
	if err != nil {
		log.Printf("Errore durante la misurazione RTT, il valore non è stato aggiornato: %v", err)
		return
	}

	m.mutex.Lock()
	m.latestRtt = newRtt
	m.mutex.Unlock()

	log.Printf("Nuova latenza RTT verso Lambda misurata: %v", newRtt)
}

func (m *RttMonitor) measure() (time.Duration, error) {
	opts := RTTBatchOpts{Attempts: 5, Warmup: 1, TrimRatio: 0.2, Timeout: 1500 * time.Millisecond}
	cli, err := newNoRetryLambdaClient(context.Background())
	if err != nil {
		return 0, fmt.Errorf("impossibile creare il client Lambda: %w", err)
	}

	if err := EnsurePingFunction(context.Background(), m.lambdaFnName); err != nil {
		return 0, fmt.Errorf("fallimento nel verificare la funzione di ping: %w", err)
	}

	vals := make([]time.Duration, 0, opts.Attempts)
	invokeOnce := func(ctx context.Context) (time.Duration, bool, error) {
		cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		in := &lambda.InvokeInput{
			FunctionName:   aws.String(m.lambdaFnName),
			InvocationType: types.InvocationTypeRequestResponse,
			LogType:        types.LogTypeTail,
			Payload:        []byte(`{"ping":true}`),
		}
		start := time.Now()
		out, err := cli.Invoke(cctx, in)
		if err != nil {
			return 0, false, err
		}
		total := time.Since(start)

		if _, isCold := utils.ExtractInitDurationFromLog(out.LogResult); isCold {
			return 0, true, nil
		}
		var handler float64
		if d, ok := utils.ExtractDurationFromLog(out.LogResult); ok {
			handler = d
		}
		transport := total - time.Duration(handler*float64(time.Millisecond))
		return transport, false, nil
	}

	for i := 0; i < opts.Attempts; i++ {
		d, cold, err := invokeOnce(context.Background())
		if err != nil || cold || i < opts.Warmup {
			continue
		}
		vals = append(vals, d)
	}

	if len(vals) == 0 {
		return 0, fmt.Errorf("nessun campione 'warm' raccolto durante la misurazione RTT")
	}

	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
	trim := int(float64(len(vals)) * opts.TrimRatio)
	if 2*trim < len(vals) {
		vals = vals[trim : len(vals)-trim]
	}
	mid := len(vals) / 2
	if len(vals)%2 == 1 {
		return vals[mid], nil
	}
	return (vals[mid-1] + vals[mid]) / 2, nil
}

func EnsurePingFunction(ctx context.Context, fnName string) error {
	// It already exists?
	provider, err := GetProvider()
	if err != nil {
		return fmt.Errorf("error getting provider: %v", err)
	}
	list, _ := provider.ListFunctions(ctx)

	for _, name := range list {
		if name == fnName {
			return nil
		}
	}

	//If it doesn't exist, we create it
	code := `def lambda_handler(event, context): return {"pong": True}`
	zipBytes, _ := CreateZipFromCode(code, "lambda_function.py")

	input := &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Runtime:      types.RuntimePython310,
		Role:         aws.String(RolePolicy),
		Handler:      aws.String("lambda_function.lambda_handler"),
		Code: &types.FunctionCode{
			ZipFile: zipBytes,
		},
		MemorySize: aws.Int32(128),
		Timeout:    aws.Int32(3),
		Publish:    true,
	}

	_, err = provider.client.CreateFunction(ctx, input)

	if err != nil {
		return fmt.Errorf("error creating function: %v", err)
	}

	return nil
}

func newNoRetryLambdaClient(ctx context.Context) (*lambda.Client, error) {
	base, err := utils.LoadAWSConfig()
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(base.Region),
		config.WithCredentialsProvider(base.Credentials),
		config.WithRetryer(func() aws.Retryer { return aws.NopRetryer{} }), // 0 retry
	)
	if err != nil {
		return nil, err
	}
	return lambda.NewFromConfig(cfg), nil
}
