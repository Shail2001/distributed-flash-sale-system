# Flash Sale Platform — 2-Minute Elevator Pitch Script
## CS6650 Milestone 1 | Shail Shah, Vikas Neriyanuru, Darshan Ravindra Konnur

---

## TIMING GUIDE
- Intro & problem: 0:00–0:25
- Architecture: 0:25–0:55
- Experiments: 0:55–1:30
- Team & progress: 1:30–1:50
- Close: 1:50–2:00

---

## SCRIPT

**[0:00 — Hook]**
"Every year, product drops — limited sneakers, concert tickets, Black Friday deals — crash
the systems they run on. Not because of bad hardware, but because of a single, classic
distributed systems problem: two users try to buy the last item at the exact same time.
Who gets it? Who gets oversold? Our project answers that."

**[0:15 — Problem statement]**
"We're building a flash sale platform on AWS that can handle thousands of concurrent
purchase requests without overselling a single unit. The core challenge is atomic inventory
management — guaranteeing exactly-once decrement across multiple service replicas."

**[0:25 — Architecture walkthrough]** *(show diagram or slide)*
"Our architecture is three Go microservices behind an AWS ALB.
First, a Waiting Room that throttles traffic and issues timed tokens.
Second, the Flash Sale API — this is what I built — which uses Redis ElastiCache for
atomic inventory control and publishes accepted orders to SQS.
Third, an Order Worker that consumes from SQS and persists confirmed orders to DynamoDB.
Everything runs on ECS Fargate, provisioned with Terraform."

**[0:50 — The key insight]**
"The secret is a single Redis DECR command. Because Redis is single-threaded, that
operation is atomic across all our API replicas. No locks. No transactions. No
read-check-write race conditions. Just DECR, check the result, and if it's negative —
you're sold out."

**[1:00 — Experiments]**
"We're running three experiments to quantify the tradeoffs.
Experiment 1 measures throughput and latency under load — how many purchases per second
can we handle before p99 latency degrades?
Experiment 2 compares three inventory strategies: our atomic DECR, optimistic locking,
and a Lua script approach — to see which prevents oversell most reliably under contention.
Experiment 3 sweeps the Order Worker's goroutine count — 20, 40, and 80 workers — to
find the optimal concurrency for SQS processing throughput."

**[1:30 — Team & current progress]**
"Our team brings together distributed systems, infrastructure, and backend engineering.
Shail is driving Terraform infrastructure and the Waiting Room service.
Vikas — that's me — built the Flash Sale API, which is fully implemented and ready to deploy.
Darshan is building the Order Worker.
We have the GitHub repo set up, infrastructure provisioning underway, and our first
preliminary results from local smoke tests."

**[1:50 — Close]**
"By the end of this project, we'll have concrete, data-backed answers to a problem every
high-traffic e-commerce system faces. The platform will be live on AWS — and we'd love
for classmates to stress-test it with us. Stay tuned."

---

## DELIVERY TIPS
- Record in one take if possible — 2 minutes is short, practice 2–3 times first
- Use screen share to show the architecture diagram while you talk through it (0:25–0:50)
- Keep energy up at the hook — the "two users, last item" framing is your best opener
- You can use the README diagram or draw a quick boxes-and-arrows slide in Canva/Google Slides
- Film in landscape, good lighting, AirPods or a headset for audio quality
