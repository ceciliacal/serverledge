# parser.py
import ast, json, sys, textwrap

def func_body_source(code: str, node: ast.AST) -> str:
    if not getattr(node, "body", None):
        return ""

    first = node.body[0]
    last  = node.body[-1]

    # Calcola slicing per righe
    lines = code.splitlines(keepends=True)
    start = first.lineno - 1
    end   = getattr(last, "end_lineno", last.lineno)  # fallback
    snippet = "".join(lines[start:end])

    # De-indenta un livello (quello della funzione)
    return textwrap.dedent(snippet)

def parse_functions(code: str):
    tree = ast.parse(code)
    results = []
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            params = [a.arg for a in node.args.args]
            results.append({
                "name": node.name,
                "params": params,
                "docstring": ast.get_docstring(node) or "",
                "is_async": isinstance(node, ast.AsyncFunctionDef),
                "body": func_body_source(code, node),
            })
    return results

if __name__ == "__main__":
    code = sys.stdin.read()
    print(json.dumps(parse_functions(code)))
