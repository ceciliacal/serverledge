def handler(params, context):
    """
    Esempio di handler:
    - Calcola la somma dei numeri primi fino a n
    - Gestisce parametri opzionali
    - Restituisce più campi nell'output
    """
    try:
        n = int(params.get("n", 10))  # default = 10 se non fornito
        verbose = params.get("verbose", False)

        if n < 2:
            return {"error": "n deve essere >= 2"}

        primes = generate_primes(n)
        prime_sum = sum(primes)

        if verbose:
            print(f"[DEBUG] Lista primi fino a {n}: {primes}")

        result = {
            "InputN": n,
            "PrimeCount": len(primes),
            "PrimeSum": prime_sum,
            "MaxPrime": primes[-1] if primes else None
        }
        return result

    except ValueError:
        return {"error": "Parametro n non è un intero valido"}
    except Exception as e:
        return {"error": str(e)}


def generate_primes(limit):
    """Genera tutti i numeri primi fino a 'limit'"""
    primes = []
    for num in range(2, limit + 1):
        if is_prime(num):
            primes.append(num)
    return primes


def is_prime(n):
    """Controllo primalità con ottimizzazione semplice"""
    if n < 2:
        return False
    if n in (2, 3):
        return True
    if n % 2 == 0 or n % 3 == 0:
        return False
    i = 5
    while i * i <= n:
        if n % i == 0 or n % (i + 2) == 0:
            return False
        i += 6
    return True
