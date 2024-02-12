import datetime

from locust import HttpUser, task, between, tag, constant_throughput


class QuickstartUser(HttpUser):
    # wait_time = between(1, 5)   #seconds
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
        #json_response_dict = response.json()
        #print(json_response_dict)
        if response.status_code == 410:
            response.failure("fine batteria ",datetime.datetime.now())
        return response


'''
    @tag('func')
    @task()
    def euler_method(self):
        response= self.client.post("/invoke/func",
                                   json={ "Params": {},"QoSClass": 0,"QoSMaxRespT": -1,"CanDoOffloading": True,"Async": False} )
        json_response_dict=response.json()
        print(json_response_dict)
        print(json_response_dict['Result'])
        return response
'''
'''
    @tag('euler_method')
    @task
    def hello_world(self):
        self.client.get("/hello")
        self.client.get("/world")
        
    def on_start(self):
        self.client.post("/login", json={"username": "foo", "password": "bar"})
'''
