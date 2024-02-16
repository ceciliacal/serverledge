import tempfile

import imageio
import requests
import tensorflow as tf
from tensorflow import keras

modelR50 = keras.applications.resnet50.ResNet50(weights="imagenet")
modelR152 = keras.applications.ResNet152(weights="imagenet")

def predictResNet50 (input_img):
    image = imageio.imread(input_img)
    resized = tf.image.resize([image], (224,224))
    inputs = keras.applications.resnet.preprocess_input(resized)
    return modelR50.predict(inputs)

def predictResNet152 (input_img):
    image = imageio.imread(input_img)
    resized = tf.image.resize([image], (224,224))
    inputs = keras.applications.resnet.preprocess_input(resized)
    return modelR152.predict(inputs)

    
def prob2class(Y):
    top_K = keras.applications.resnet50.decode_predictions(Y, top=1)
    for class_id, name, y_proba in top_K[0]:
        return name

def handler (params, context):
    # se hai problemi con i parametri, puoi anche inserire qua un URL
    # hard-coded:
    image_url = "https://upload.wikimedia.org/wikipedia/commons/thumb/c/c5/Edvard_Munch%2C_1893%2C_The_Scream%2C_oil%2C_tempera_and_pastel_on_cardboard%2C_91_x_73_cm%2C_National_Gallery_of_Norway.jpg/800px-Edvard_Munch%2C_1893%2C_The_Scream%2C_oil%2C_tempera_and_pastel_on_cardboard%2C_91_x_73_cm%2C_National_Gallery_of_Norway.jpg"
    print("Downloading: " + image_url)
    r = requests.get(image_url)
    with tempfile.NamedTemporaryFile() as of:
        of.write(r.content)
        of.flush()
        input_file=of.name

        # oppure, SoC alta:
        y=predictResNet152(input_file)

        prediction = prob2class(y)
        return {"Class": prediction}
