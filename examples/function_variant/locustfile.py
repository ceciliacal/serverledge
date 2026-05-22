import os

from locust import HttpUser, between, task


DEFAULT_IMAGE_URL = "https://upload.wikimedia.org/wikipedia/commons/d/de/Nokota_Horses_cropped.jpg"


def env_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "y", "on"}


def env_int(name: str, default: int) -> int:
    value = os.getenv(name)
    if value is None or value.strip() == "":
        return default
    return int(value)


def env_float(name: str, default: float) -> float:
    value = os.getenv(name)
    if value is None or value.strip() == "":
        return default
    return float(value)


QOS_CLASS = env_int("SERVERLEDGE_QOS_CLASS", 1)
QOS_MAX_RESP_T = env_float("SERVERLEDGE_QOS_MAX_RESP_T", 30.0)
CAN_DO_OFFLOADING = env_bool("SERVERLEDGE_CAN_DO_OFFLOADING", True)
ASYNC = env_bool("SERVERLEDGE_ASYNC", False)
RETURN_OUTPUT = env_bool("SERVERLEDGE_RETURN_OUTPUT", False)
ML_IMAGE_URL = os.getenv("SERVERLEDGE_ML_IMAGE_URL", DEFAULT_IMAGE_URL)


FUNCTION_NAMES = {
    "video": os.getenv("SERVERLEDGE_VIDEO_FUNCTION", "video_standard"),
    "euler": os.getenv("SERVERLEDGE_EULER_FUNCTION", "euler_standard"),
    "ml": os.getenv("SERVERLEDGE_ML_FUNCTION", "ml_standard"),
}


def invocation_payload(params: dict | None = None) -> dict:
    return {
        "Params": params or {},
        "QoSClass": QOS_CLASS,
        "QoSMaxRespT": QOS_MAX_RESP_T,
        "CanDoOffloading": CAN_DO_OFFLOADING,
        "Async": ASYNC,
        "ReturnOutput": RETURN_OUTPUT,
    }


class ServerledgeFunctionUser(HttpUser):
    abstract = True
    wait_time = between(
        env_float("SERVERLEDGE_WAIT_MIN", 1.0),
        env_float("SERVERLEDGE_WAIT_MAX", 5.0),
    )

    function_key = ""
    params = {}

    @task
    def invoke(self):
        function_name = FUNCTION_NAMES[self.function_key]
        with self.client.post(
            f"/invoke/{function_name}",
            json=invocation_payload(self.params),
            name=f"/invoke/{self.function_key}",
            catch_response=True,
        ) as response:
            if response.status_code == 200:
                return
            response.failure(
                f"{function_name} returned {response.status_code}: {response.text[:500]}"
            )


class EulerUser(ServerledgeFunctionUser):
    function_key = "euler"


class MLUser(ServerledgeFunctionUser):
    function_key = "ml"
    params = {"image_url": ML_IMAGE_URL}


class VideoUser(ServerledgeFunctionUser):
    function_key = "video"


class SelectedFunctionUser(ServerledgeFunctionUser):
    function_key = os.getenv("SERVERLEDGE_FUNCTION", "euler").strip().lower()
    params = {"image_url": ML_IMAGE_URL} if function_key == "ml" else {}

    def on_start(self):
        if self.function_key not in FUNCTION_NAMES:
            supported = ", ".join(sorted(FUNCTION_NAMES))
            raise ValueError(
                f"Unsupported SERVERLEDGE_FUNCTION={self.function_key!r}; "
                f"expected one of: {supported}"
            )
