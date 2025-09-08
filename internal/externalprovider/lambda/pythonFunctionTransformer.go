package lambda

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/serverledge-faas/serverledge/internal/function"
)

type PythonFunction struct {
	Name      string
	Params    []string
	Body      string `json:"body"`
	Docstring string
	IsHandler bool
}

func transformServerledgeToAWSLambda(fn function.Function, tarCode []byte) (string, error) {
	pythonCode, err := extractPythonFromTar(tarCode)
	if err != nil {
		return "", fmt.Errorf("failed to extract Python Code: %v", err)
	}

	// Parse .py function
	functions, err := parsePythonFunctions(pythonCode)
	if err != nil {
		return "", fmt.Errorf("failed to parse Python functions: %v", err)
	}

	fmt.Println("3")

	for _, funct := range functions {
		fmt.Printf("Funzione: %s, Params: %v\n", funct.Name, funct.Params)
	}

	// Generating AWS Code
	log.Printf("Handler %s", fn.Handler)
	awsCode := generateAWSCode(functions, fn.Handler)

	return awsCode, nil
}

func parsePythonFunctions(code string) ([]PythonFunction, error) {

	parserPath := "internal/externalprovider/lambda/parser.py"

	// 2) -u = unbuffered; CombinedOutput cattura anche stderr
	cmd := exec.Command("python3", "-u", parserPath)

	// 3) Passa il codice via Stdin senza goroutine/pipe manuale
	cmd.Stdin = strings.NewReader(code)

	// 4) Cattura stdout+stderr
	out, err := cmd.CombinedOutput()
	if err != nil {
		// out contiene anche stderr → molto utile per capire perché fallisce
		return nil, fmt.Errorf("parser failed: %v\nstderr/stdout:\n%s", err, string(out))
	}

	// 5) Decodifica JSON
	var functions []PythonFunction
	if jerr := json.Unmarshal(out, &functions); jerr != nil {
		return nil, fmt.Errorf("error decoding JSON: %w\nraw:\n%s", jerr, string(out))
	}
	return functions, nil
}

// helper: estrae il nome della funzione dall’handler spec "mod.path.func"
// se non c’è '.', ritorna la stringa intera (meglio di niente)
func handlerFuncName(spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	if i := strings.LastIndex(spec, "."); i >= 0 && i < len(spec)-1 {
		return spec[i+1:]
	}
	return spec
}

func generateAWSCode(functions []PythonFunction, handler2 string) string {

	wantHandler := handlerFuncName(handler2) // es. "handler"

	var b strings.Builder
	indent := func(s string, n int) string {
		s = strings.TrimRight(s, "\n")
		if strings.TrimSpace(s) == "" {
			return strings.Repeat(" ", n) + "pass\n"
		}
		return addIndentation(s, n) + "\n"
	}

	// 1) Seleziona l'handler
	var handler *PythonFunction
	if wantHandler != "" {
		for i := range functions {
			if strings.TrimSpace(functions[i].Name) == wantHandler {
				handler = &functions[i]
				break
			}
		}
	}
	if handler == nil {
		for i := range functions {
			if functions[i].IsHandler {
				handler = &functions[i]
				break
			}
		}
	}
	if handler == nil {
		for i := range functions {
			if strings.TrimSpace(functions[i].Name) == "handler" {
				handler = &functions[i]
				break
			}
		}
	}
	if handler == nil {
		for i := range functions {
			ps := functions[i].Params
			if len(ps) == 2 {
				p0 := strings.TrimSpace(ps[0])
				p1 := strings.TrimSpace(ps[1])
				if (p0 == "context" && p1 == "params") || (p0 == "params" && p1 == "context") {
					handler = &functions[i]
					break
				}
			}
		}
	}

	// 2) Header
	b.WriteString("# Generated AWS Lambda function from Serverledge code\n")

	// 3) Stampa tutte le funzioni NON handler come supporto
	for _, f := range functions {
		if handler != nil && f.Name == handler.Name {
			continue // non ristampare col nome originale
		}
		b.WriteString(fmt.Sprintf("def %s(%s):\n", f.Name, strings.Join(f.Params, ", ")))
		b.WriteString(indent(f.Body, 4))
		b.WriteString("\n")
	}

	// 4) Rinomina l’handler originale in serverledge_<name>
	if handler != nil {
		b.WriteString(fmt.Sprintf("def serverledge_%s(%s):\n", handler.Name, strings.Join(handler.Params, ", ")))
		b.WriteString(indent(handler.Body, 4))
		b.WriteString("\n")
	}

	// 5) AWS entrypoint *fisso*: lambda_handler
	b.WriteString(fmt.Sprintf("def %s(event, context):\n", awsHandlerName))
	b.WriteString("    \"\"\"\n")
	b.WriteString("    AWS Lambda handler transformed from Serverledge code\n")
	b.WriteString("    \"\"\"\n")
	b.WriteString("    try:\n")
	b.WriteString("        # Extract parameters from AWS event format\n")
	b.WriteString("        params = event.get(\"Params\", {})\n")

	if handler != nil {
		call := "serverledge_%s(params, context)"
		if len(handler.Params) >= 2 {
			p0 := strings.TrimSpace(handler.Params[0])
			p1 := strings.TrimSpace(handler.Params[1])
			if p0 == "context" && p1 == "params" {
				call = "serverledge_%s(context, params)"
			}
		}
		b.WriteString(fmt.Sprintf("        return "+call+"\n", handler.Name))
	} else {
		b.WriteString("        raise Exception(\"No original Serverledge handler found\")\n")
	}

	b.WriteString("    except Exception as e:\n")
	b.WriteString("        print(f\"Error: {e}\")\n")
	b.WriteString("        return {\"error\": str(e)}\n")

	log.Print(b.String())

	return b.String()
}

func addIndentation(code string, spaces int) string {
	indentation := strings.Repeat(" ", spaces)
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = indentation + line
		}
	}
	return strings.Join(lines, "\n")
}

// extractPythonFromTar estrae il codice Python da bytes TAR
func extractPythonFromTar(tarBytes []byte) (string, error) {
	var pythonCode strings.Builder
	tarReader := tar.NewReader(bytes.NewReader(tarBytes))

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Processa solo file Python
		if strings.HasSuffix(header.Name, ".py") {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return "", err
			}
			pythonCode.WriteString(string(content))
			pythonCode.WriteString("\n")
		}
	}

	return pythonCode.String(), nil
}
