import tensorflow as tf
from tensorflow import keras
from tensorflow.keras.applications import resnet50, resnet, mobilenet_v2

import numpy as np
import requests


# --- Load models once at import time (cold start cost only) ---

modelR50 = resnet50.ResNet50(weights="imagenet")
modelR152 = resnet.ResNet152(weights="imagenet")
modelMob = mobilenet_v2.MobileNetV2(weights="imagenet")

def load_and_preprocess_image_from_bytes(img_bytes: bytes) -> tf.Tensor:
    img = tf.io.decode_image(img_bytes, channels=3, expand_animations=False)
    img = tf.image.resize(img, (224, 224))
    img = tf.cast(img, tf.float32)
    return img[None, ...]


def predictResNet50(image_batch: tf.Tensor) -> np.ndarray:
    inputs = resnet50.preprocess_input(tf.identity(image_batch))
    return modelR50.predict(inputs)


def predictResNet152(image_batch: tf.Tensor) -> np.ndarray:
    inputs = resnet.preprocess_input(tf.identity(image_batch))
    return modelR152.predict(inputs)


def predictMobileNet(image_batch: tf.Tensor) -> np.ndarray:
    inputs = mobilenet_v2.preprocess_input(tf.identity(image_batch))
    return modelMob.predict(inputs)


def prob2class(Y: np.ndarray) -> str:
    top_K = resnet50.decode_predictions(Y, top=1)
    class_id, name, y_proba = top_K[0][0]
    return name


# --- Serverledge handler ---

def handler(params, context):
    # Allow overriding the URL via params; fallback to your horse image
    default_url = "https://upload.wikimedia.org/wikipedia/commons/d/de/Nokota_Horses_cropped.jpg"
    if isinstance(params, dict) and "image_url" in params:
        image_url = params["image_url"]
    else:
        image_url = default_url

    print("Downloading:", image_url, flush=True)
    headers = {
        "User-Agent": "Serverledge-ML/1.0 (https://example.com)"
    }

    try:
        resp = requests.get(image_url, headers=headers, timeout=15)
        resp.raise_for_status()
    except requests.RequestException as e:
        print(f"Error downloading {image_url}: {e}", flush=True)
        return {"Class": None, "Error": f"download_failed: {e}"}


    # Load and preprocess once, reuse for all three models
    image_batch = load_and_preprocess_image_from_bytes(resp.content)

    y0 = predictMobileNet(image_batch)
    y1 = predictResNet50(image_batch)
    y2 = predictResNet152(image_batch)
    y = (y0 + y1 + y2) / 3.0

    prediction = prob2class(y)
    return {"Class": prediction}
