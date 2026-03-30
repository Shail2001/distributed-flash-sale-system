output "cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "waiting_room_service_name" {
  value = aws_ecs_service.waiting_room.name
}

output "flash_sale_api_service_name" {
  value = aws_ecs_service.flash_sale_api.name
}

output "order_worker_service_name" {
  value = aws_ecs_service.order_worker.name
}
