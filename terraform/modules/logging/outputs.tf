output "waiting_room_log_group" {
  value = aws_cloudwatch_log_group.waiting_room.name
}

output "flash_sale_api_log_group" {
  value = aws_cloudwatch_log_group.flash_sale_api.name
}

output "order_worker_log_group" {
  value = aws_cloudwatch_log_group.order_worker.name
}
