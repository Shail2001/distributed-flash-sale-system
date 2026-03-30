output "redis_endpoint" {
  description = "Redis primary endpoint for application connection"
  value       = aws_elasticache_replication_group.this.primary_endpoint_address
}

output "redis_port" {
  value = 6379
}
