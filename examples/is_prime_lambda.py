def handler(event, context):
    try:

        params = event.get("Params", {})
        raw = params.get("n")
        if raw is None:
            raise ValueError("Missing 'n' inside event['Params']")

        n = int(raw)
        print(f"Checking n = {n}")
        return {"IsPrime": is_prime(n)}

    except Exception as e:
        print(f"Error: {e}")
        return {"error": str(e)}



def is_prime(n):
    if n < 2:
        return False
    if n == 2:
        return True
    if n % 2 == 0:
        return False

    i = 3
    while i * i <= n:
        if n % i == 0:
            return False
        i += 2
    return True
