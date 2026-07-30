import subprocess
import base64
import os
import sys

def handler(params, context):
    input_path = "/app/video.mp4"
    output_path = "/tmp/output.mp4"

    print(f"[video_light] Using input {input_path}", flush=True)

    cmd = [
        "ffmpeg",
        "-threads", "1",
        "-y",
        "-i", input_path,
        "-vcodec", "h264",
        "-vf", "scale=320:240",
        "-threads", "1",
        "-acodec", "mp2",
        output_path,
    ]

    proc = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    print(proc.stderr.decode(), file=sys.stderr, flush=True)

    if proc.returncode != 0:
        raise RuntimeError("[video_light] ffmpeg failed, see stderr above")

    if not os.path.exists(output_path):
        raise FileNotFoundError("[video_light] output.mp4 was not created by ffmpeg")

    size = os.path.getsize(output_path)
    print(f"[video_light] Created {output_path}, size={size} bytes", flush=True)

    # Return a small, clean JSON result
    return {
        "variant" : "light",
        "status": "ok",
        "output_size_bytes": size,
    }
