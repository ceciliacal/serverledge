FROM python:3.10-bookworm

WORKDIR /app

RUN apt-get update \
    && apt-get install -y ffmpeg \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir requests

COPY executor.py /app/
COPY video.mp4 /app/video.mp4

CMD ["python", "executor.py"]
