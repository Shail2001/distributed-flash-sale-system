import json
import os
import random
import uuid

from locust import HttpUser, between, task


DEFAULT_QUANTITY = int(os.getenv("PURCHASE_QUANTITY", "1"))


class FlashSaleUser(HttpUser):
    wait_time = between(0.001, 0.05)

    @task(1)
    def get_inventory(self):
        self.client.get("/inventory", name="GET /inventory")

    @task(4)
    def purchase(self):
        body = {
            "customer_id": f"locust-{uuid.uuid4()}",
            "quantity": DEFAULT_QUANTITY,
        }
        with self.client.post(
            "/purchase",
            name="POST /purchase",
            data=json.dumps(body),
            headers={"Content-Type": "application/json"},
            catch_response=True,
        ) as response:
            if response.status_code not in (201, 409):
                response.failure(f"unexpected status={response.status_code} body={response.text}")
            else:
                response.success()

    @task(1)
    def jitter(self):
        # Small random no-op load pattern helps mimic mixed client behavior.
        _ = random.randint(1, 1000)
