import datetime

from locust import HttpUser, task, constant


class ReadBatteryNode(HttpUser):
    wait_time = constant(30)

    @task()
    def read_battery(self):
        response = self.client.get("/status")
        f = open('battery_stats.txt', 'a')
        f.write(str(datetime.datetime.now())+","+str(response.json()["SoC"])+"\n")
        f.close()
        return response
