output "waiting_room_repo_url" {
  value = aws_ecr_repository.waiting_room.repository_url
}

output "flash_sale_api_repo_url" {
  value = aws_ecr_repository.flash_sale_api.repository_url
}

output "order_worker_repo_url" {
  value = aws_ecr_repository.order_worker.repository_url
}
