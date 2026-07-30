import subprocess
import requests
import base64

URL = "https://backupgrr.s3.eu-central-1.amazonaws.com/video/BigBuckBunnyShort.mp4"
response = requests.get(URL)
open("video.mp4", "wb").write(response.content)

HIGH_QUALITY=False

if HIGH_QUALITY:
    subprocess.run(['ffmpeg', '-threads', '1', '-y', '-i', 'video.mp4', '-vcodec', 'h264', '-vf', 'scale=640:480', '-threads', '1', '-preset', 'slow', '-crf', '5', '-acodec', 'mp2', 'output.mp4'])
else:
    subprocess.run(['ffmpeg', '-threads', '1', '-y', '-i', 'video.mp4', '-vcodec', 'h264', '-vf', 'scale=320:240',  '-threads', '1', '-acodec', 'mp2', 'output.mp4'])

with open("output.mp4", "rb") as f:
    encoded_string = base64.b64encode(f.read())
    result = {"Video": encoded_string}
    # TODO: return result


