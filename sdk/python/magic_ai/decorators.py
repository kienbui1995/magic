def capability(name: str, description: str = "", est_cost: float = 0.0):
    def decorator(func):
        func._magic_capability = {
            "name": name,
            "description": description or func.__doc__ or "",
            "est_cost_per_call": est_cost,
        }
        return func
    return decorator
