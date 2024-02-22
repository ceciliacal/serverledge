import tensorflow as tf
from tensorflow import keras

import tempfile
import numpy as np
import os
import imageio
import requests

modelR50 = keras.applications.resnet50.ResNet50(weights="imagenet")
modelR152 = keras.applications.ResNet152(weights="imagenet")
modelMob = keras.applications.MobileNetV2(weights="imagenet")


def predictResNet50(input_img):
    image = imageio.imread(input_img)
    resized = tf.image.resize([image], (224, 224))
    inputs = keras.applications.resnet.preprocess_input(resized)
    return modelR50.predict(inputs)


def predictResNet152(input_img):
    image = imageio.imread(input_img)
    resized = tf.image.resize([image], (224, 224))
    inputs = keras.applications.resnet.preprocess_input(resized)
    return modelR152.predict(inputs)


def predictMobileNet(input_img):
    image = imageio.imread(input_img)
    resized = tf.image.resize([image], (224, 224))
    inputs = keras.applications.mobilenet.preprocess_input(resized)
    return modelMob.predict(inputs)


def prob2class(Y):
    top_K = keras.applications.resnet50.decode_predictions(Y, top=1)
    for class_id, name, y_proba in top_K[0]:
        return name


def handler(params, context):
    # se hai problemi con i parametri, puoi anche inserire qua un URL
    # hard-coded:
    #image_url = params["imgurl"]
    image_url = "https://upload.wikimedia.org/wikipedia/commons/d/de/Nokota_Horses_cropped.jpg"

    print("Downloading: " + image_url)
    r = requests.get(image_url)
    with tempfile.NamedTemporaryFile() as of:
        of.write(r.content)
        of.flush()
        input_file = of.name

        # SoC bassa:
        # y = predictMobileNet(input_file)

        # oppure, SoC alta:
        y0 = predictMobileNet(input_file)
        y1 = predictResNet50(input_file)
        y2 = predictResNet152(input_file)
        y = (y0 + y1 + y2) / 3.0

        prediction = prob2class(y)
        return {"Class": prediction}


params = {"imgurl": "https://upload.wikimedia.org/wikipedia/commons/d/de/Nokota_Horses_cropped.jpg"}

