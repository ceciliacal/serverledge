import datetime

from locust import HttpUser, task, tag, constant_throughput


class QuickstartUser(HttpUser):
    wait_time = constant_throughput(1 / 120)

    @tag('euler')
    @task()
    def euler_method(self):
        response = self.client.post("/invoke/euler",
                                    json={"Params": {},
                                          "QoSClass": 0,
                                          "QoSMaxRespT": -1,
                                          "CanDoOffloading": True,
                                          "Async": False,
                                          "SoC": 0.0})

        f = open('euler_light_stats.txt', 'a')
        f.write(str(datetime.datetime.now())+","+str(response.json())+"\n")
        f.close()
        if response.status_code == 410:
            response.failure("fine batteria ",datetime.datetime.now())
        return response


