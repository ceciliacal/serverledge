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

const parserPath = "internal/externalprovider/lambda/utils/parser.py"

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

	for _, funct := range functions {
		fmt.Printf("Funzione: %s, Params: %v\n", funct.Name, funct.Params)
	}

	// Generating AWS Code
	awsCode := generateAWSCode(functions, fn.Handler)

	return awsCode, nil
}

func parsePythonFunctions(code string) ([]PythonFunction, error) {

	// 2) -u = unbuffered; CombinedOutput cattura anche stderr
	cmd := exec.Command("python3", "-u", parserPath)

	cmd.Stdin = strings.NewReader(code)

	out, err := cmd.CombinedOutput()
	if err != nil {
		// out contiene anche stderr → molto utile per capire perché fallisce
		return nil, fmt.Errorf("parser failed: %v\nstderr/stdout:\n%s", err, string(out))
	}

	var functions []PythonFunction
	if jerr := json.Unmarshal(out, &functions); jerr != nil {
		return nil, fmt.Errorf("error decoding JSON: %w\nraw:\n%s", jerr, string(out))
	}
	return functions, nil
}

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

func generateAWSCode(functions []PythonFunction, handlerFunction string) string {
	wantHandler := handlerFuncName(handlerFunction) // estrae solo il nome puro

	// cerca quel nome
	var handler *PythonFunction
	for i := range functions {
		if strings.TrimSpace(functions[i].Name) == wantHandler {
			handler = &functions[i]
			break
		}
	}

	if handler == nil {
		log.Printf("Handler %q non trovato", wantHandler)
		return ""
	}

	var b strings.Builder
	indent := func(s string, n int) string {
		s = strings.TrimRight(s, "\n")
		if strings.TrimSpace(s) == "" {
			return strings.Repeat(" ", n) + "pass\n"
		}
		return addIndentation(s, n) + "\n"
	}

	// header
	b.WriteString("# Generated AWS Lambda function from Serverledge code\n")

	// funzioni di supporto
	for _, f := range functions {
		if f.Name == handler.Name {
			continue
		}
		b.WriteString(fmt.Sprintf("def %s(%s):\n", f.Name, strings.Join(f.Params, ", ")))
		b.WriteString(indent(f.Body, 4))
		b.WriteString("\n")
	}

	// handler rinominato
	b.WriteString(fmt.Sprintf("def serverledge_%s(%s):\n", handler.Name, strings.Join(handler.Params, ", ")))
	b.WriteString(indent(handler.Body, 4))
	b.WriteString("\n")

	// entrypoint AWS fisso
	b.WriteString("def lambda_handler(event, context):\n")
	b.WriteString("    \"\"\"AWS Lambda handler transformed from Serverledge code\"\"\"\n")
	b.WriteString("    try:\n")
	b.WriteString("        params = event.get(\"Params\", {})\n")
	b.WriteString(fmt.Sprintf("        return serverledge_%s(params, context)\n", handler.Name))
	b.WriteString("    except Exception as e:\n")
	b.WriteString("        print(f\"Error: {e}\")\n")
	b.WriteString("        return {\"error\": str(e)}\n")

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
