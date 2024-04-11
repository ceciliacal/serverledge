import numpy as np


def euler_method(df, x0, h, N):
    """Solves an ODE IVP using the Explicit Euler method.
    Keyword arguments:
    df  - The derivative of the system you wish to solve.
    x0 - The initial value of the system you wish to solve.
    h  - The step size.
    N  - The number off steps.
    """
    x = np.zeros(N)
    t, x[0] = x0
    for i in range(0, N - 1):
        x[i + 1] = x[i] + h * df(t, x[i])
        t += h

    return x


def df(t, y):
    return -0.5 * np.exp(t * 0.5) * np.sin(5 * t) + 5 * np.exp(t * 0.5) * np.cos(5 * t) + y


def handler(params, context):
    is_light_variant = bool(context['isLightVariant'])

    if not is_light_variant:
        h = 0.00001
    else:
        h = 0.001

    f0 = (0, 0)
    tn = 13
    N = int(tn / h)
    euler_method(df, f0, h, N)
    return ''.join("Explicit Euler method with step size h : " + str(h))

'''
 batteryLevel = params["SoC"]
    if batteryLevel > 40.0:
        h = 0.00001
        f0 = (0, 0)
        tn = 13
        N = int(tn / h)
        euler_method(df, f0, h, N)
        return ''.join("Standard energy consumpation version - Explicit Euler method with step size h : " + str(h))
    else:
        h = 0.001
        f0 = (0, 0)
        tn = 13
        N = int(tn / h)
        euler_method(df, f0, h, N)
        return ''.join("Lower energy consumpation version - Explicit Euler method with step size h : " + str(h))

'''
