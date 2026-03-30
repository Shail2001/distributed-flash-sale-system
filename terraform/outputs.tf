output "alb_dns_name" {
  description = "ALB DNS — set this as Locust host: http://<value>"
  value       = module.alb.alb_dns_name
}

output "redis_endpoint" {
  description = "Redis endpoint — inject into services via env vars"
  value       = module.redis.redis_endpoint
}

output "orders_queue_url" {
  description = "SQS queue URL — used by Flash Sale API and Order Worker"
  value       = module.sqs.orders_queue_url
}

output "dlq_url" {
  description = "Dead Letter Queue URL — monitor for failed orders"
  value       = module.sqs.dlq_url
}

output "orders_table_name" {
  description = "DynamoDB orders table — query for consistency verification"
  value       = module.dynamodb.orders_table_name
}

output "inventory_table_name" {
  description = "DynamoDB inventory table — compare with Redis"
  value       = module.dynamodb.inventory_table_name
}

output "ecs_cluster_name" {
  description = "ECS cluster name"
  value       = module.ecs.cluster_name
}

output "ecr_waiting_room_url" {
  description = "ECR URL for waiting room image push"
  value       = module.ecr.waiting_room_repo_url
}

output "ecr_api_url" {
  description = "ECR URL for flash sale API image push"
  value       = module.ecr.flash_sale_api_repo_url
}

output "ecr_worker_url" {
  description = "ECR URL for order worker image push"
  value       = module.ecr.order_worker_repo_url
}
