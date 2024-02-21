import tempfile

import imageio
import requests
import tensorflow as tf
from tensorflow import keras

import tensorflow_io as tfio


modelR50 = keras.applications.resnet50.ResNet50(weights="imagenet")
modelR152 = keras.applications.ResNet152(weights="imagenet")

def predictResNet50(input_img):
    image = imageio.imread(input_img)
    resized = tfio.image.resize([image], (224, 224))
    inputs = keras.applications.resnet.preprocess_input(resized)
    return modelR50.predict(inputs)

def predictResNet152(input_img):
    image = imageio.imread(input_img)
    resized = tfio.image.resize([image], (224, 224))
    inputs = keras.applications.resnet.preprocess_input(resized)
    return modelR152.predict(inputs)


def prob2class(Y):
    top_K = keras.applications.resnet50.decode_predictions(Y, top=1)
    for class_id, name, y_proba in top_K[0]:
        return name

def handler(params, context):
    # se hai problemi con i parametri, puoi anche inserire qua un URL
    # hard-coded:
    # image_url = params["imgurl"]

    image_url = "https://img.freepik.com/free-vector/beautiful-home_24877-50819.jpg"

    batteryLevel = params["SoC"]

    print("Downloading: " + image_url)
    r = requests.get(image_url)
    with tempfile.NamedTemporaryFile() as of:
        of.write(r.content)
        of.flush()
        input_file = of.name

        # oppure, SoC alta:
        if batteryLevel > 40.0:
            y = predictResNet152(input_file)
        else:
            y = predictResNet50(input_file)

        prediction = prob2class(y)
        return {"Class": prediction}
