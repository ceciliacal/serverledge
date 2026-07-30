import tempfile

import imageio
import requests
import tensorflow as tf
from tensorflow import keras

modelMob = keras.applications.MobileNetV2(weights="imagenet")

def load_and_preprocess_image_from_bytes(img_bytes: bytes) -> tf.Tensor:
    img = tf.io.decode_image(img_bytes, channels=3, expand_animations=False)
    img = tf.image.resize(img, (224, 224))
    img = tf.cast(img, tf.float32)
    return img[None, ...]

def predictMobileNet(image_batch: tf.Tensor) -> np.ndarray:
    inputs = mobilenet_v2.preprocess_input(tf.identity(image_batch))
    return modelMob.predict(inputs)


def prob2class(Y: np.ndarray) -> str:
    top_K = resnet50.decode_predictions(Y, top=1)
    class_id, name, y_proba = top_K[0][0]
    return name


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

    y = predictMobileNet(image_batch)
    prediction = prob2class(y)
    return {"Class": prediction}



