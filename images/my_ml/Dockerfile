FROM python:3.10-slim-buster
RUN pip3 install tensorflow requests imageio tensorflow_io

WORKDIR /

COPY executor.py /
RUN mkdir -p /app

CMD python3 /executor.py
