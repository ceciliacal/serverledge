package scheduling

import (
	"testing"

	"github.com/serverledge-faas/serverledge/internal/function"
)

type recordedMetricEvent struct {
	kind     string
	function string
	value    float64
	cold     bool
}

func recordingMetricsRecorder(events *[]recordedMetricEvent) completionMetricsRecorder {
	return completionMetricsRecorder{
		addCompletedInvocation: func(functionName string, cold bool) {
			*events = append(*events, recordedMetricEvent{kind: "completed", function: functionName, cold: cold})
		},
		addFunctionDuration: func(functionName string, value float64) {
			*events = append(*events, recordedMetricEvent{kind: "duration", function: functionName, value: value})
		},
		addFunctionInitTime: func(functionName string, value float64) {
			*events = append(*events, recordedMetricEvent{kind: "init", function: functionName, value: value})
		},
		addFunctionOutputSize: func(functionName string, value float64) {
			*events = append(*events, recordedMetricEvent{kind: "output", function: functionName, value: value})
		},
	}
}

func TestCompletionMetricsDefaultExecutionUseLogicalFunctionName(t *testing.T) {
	var events []recordedMetricEvent
	notification := &completionNotification{
		funcName:         "classifier",
		physicalFuncName: "classifier",
		report: function.ExecutionReport{
			IsWarmStart: false,
			Duration:    1.25,
			InitTime:    0.2,
			Result:      "abc",
		},
	}

	recordCompletionMetricsWith(notification, recordingMetricsRecorder(&events))

	assertMetricEvent(t, events, "completed", "classifier", 0, true)
	assertMetricEvent(t, events, "duration", "classifier", 1.25, false)
	assertMetricEvent(t, events, "init", "classifier", 0.2, false)
	assertMetricEvent(t, events, "output", "classifier", 3, false)
	assertNoMetricForFunction(t, events, "classifier_fast")
}

func TestCompletionMetricsVariantExecutionUsePhysicalFunctionName(t *testing.T) {
	var events []recordedMetricEvent
	notification := &completionNotification{
		funcName:         "classifier",
		physicalFuncName: "classifier_fast",
		report: function.ExecutionReport{
			IsWarmStart: false,
			Duration:    0.4,
			InitTime:    0.08,
			Result:      "variant-result",
		},
	}

	recordCompletionMetricsWith(notification, recordingMetricsRecorder(&events))

	assertMetricEvent(t, events, "completed", "classifier_fast", 0, true)
	assertMetricEvent(t, events, "duration", "classifier_fast", 0.4, false)
	assertMetricEvent(t, events, "init", "classifier_fast", 0.08, false)
	assertMetricEvent(t, events, "output", "classifier_fast", float64(len("variant-result")), false)
	assertNoMetricForFunction(t, events, "classifier")
}

func TestCompletionMetricsKeepVariantBucketsIndependent(t *testing.T) {
	var events []recordedMetricEvent
	recorder := recordingMetricsRecorder(&events)

	recordCompletionMetricsWith(&completionNotification{
		funcName:         "classifier",
		physicalFuncName: "classifier_fast",
		report: function.ExecutionReport{
			IsWarmStart: true,
			Duration:    0.4,
			Result:      "fast",
		},
	}, recorder)
	recordCompletionMetricsWith(&completionNotification{
		funcName:         "classifier",
		physicalFuncName: "classifier_small",
		report: function.ExecutionReport{
			IsWarmStart: true,
			Duration:    0.7,
			Result:      "small",
		},
	}, recorder)

	assertMetricEvent(t, events, "duration", "classifier_fast", 0.4, false)
	assertMetricEvent(t, events, "duration", "classifier_small", 0.7, false)
	assertNoMetricForFunction(t, events, "classifier")
}

func TestCompletionMetricsUseCompletedInvocationForOutputSizeAttribution(t *testing.T) {
	var events []recordedMetricEvent
	notification := &completionNotification{
		funcName:         "unrelated_logical_request",
		physicalFuncName: "classifier_fast",
		report: function.ExecutionReport{
			IsWarmStart: true,
			Duration:    0.1,
			Result:      "payload",
		},
	}

	recordCompletionMetricsWith(notification, recordingMetricsRecorder(&events))

	assertMetricEvent(t, events, "output", "classifier_fast", float64(len("payload")), false)
	assertNoMetricForFunction(t, events, "unrelated_logical_request")
}

func TestCompletionMetricsFallbackToLogicalNameForNonVariantLegacyCompletion(t *testing.T) {
	var events []recordedMetricEvent
	notification := &completionNotification{
		funcName: "legacy",
		report: function.ExecutionReport{
			IsWarmStart: true,
			Duration:    0.6,
			Result:      "ok",
		},
	}

	recordCompletionMetricsWith(notification, recordingMetricsRecorder(&events))

	assertMetricEvent(t, events, "completed", "legacy", 0, false)
	assertMetricEvent(t, events, "duration", "legacy", 0.6, false)
	assertMetricEvent(t, events, "output", "legacy", 2, false)
}

func assertMetricEvent(t *testing.T, events []recordedMetricEvent, kind, functionName string, value float64, cold bool) {
	t.Helper()
	for _, event := range events {
		if event.kind != kind || event.function != functionName {
			continue
		}
		if kind == "completed" {
			if event.cold != cold {
				t.Fatalf("%s/%s cold = %v, want %v", kind, functionName, event.cold, cold)
			}
			return
		}
		if event.value != value {
			t.Fatalf("%s/%s value = %v, want %v", kind, functionName, event.value, value)
		}
		return
	}
	t.Fatalf("missing metric event kind=%s function=%s in %#v", kind, functionName, events)
}

func assertNoMetricForFunction(t *testing.T, events []recordedMetricEvent, functionName string) {
	t.Helper()
	for _, event := range events {
		if event.function == functionName {
			t.Fatalf("unexpected metric for %s: %#v", functionName, event)
		}
	}
}
