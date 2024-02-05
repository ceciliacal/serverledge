import numpy as np

def explicit_euler(df, x0, h, N):
    """Solves an ODE IVP using the Explicit Euler method.
    Keyword arguments:
    df  - The derivative of the system you wish to solve.
    x0 - The initial value of the system you wish to solve.
    h  - The step size.
    N  - The number off steps.
    """
    x = np.zeros(N)
    t, x[0] = x0
    for i in range(0, N-1):
        x[i+1] = x[i] + h * df(t ,x[i])
        t += h
    
    return x

def df(t, y):
    return -0.5 * np.exp(t * 0.5) * np.sin(5 * t) + 5 * np.exp(t * 0.5) * np.cos(5 * t) + y


def handler(params, context):

    batteryLevel = params["SoC"]
    if batteryLevel > 40.0 :
        h = 0.000001
        f0 = (0, 0)
        tn = 13
        N = int(tn / h)
        explicit_euler(df, f0, h, N)
        return ''.join("Standard energy consumpation version - Explicit Euler with step size h : "+str(h))
    else:
        h=0.01
        f0 = (0, 0)
        tn = 13
        N = int(tn / h)
        explicit_euler(df, f0, h, N)
        return ''.join("Lower energy consumpation version - Explicit Euler with step size h : "+str(h))

"""        
    if float(context['SoC'])<=float(context['EnergySavingSoC']): 
        h=0.01
        f0 = (0, 0)
        tn = 13
        N = int(tn / h)
        explicit_euler(df, f0, h, N)
        return ''.join("Explicit Euler with step size h : "+str(h))

    else :
          h = 0.000001
          f0 = (0, 0)
          tn = 13
          N = int(tn / h)
          explicit_euler(df, f0, h, N)
          return ''.join("Explicit Euler with step size h : "+str(h))
"""
