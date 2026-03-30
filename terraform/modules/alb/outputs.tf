output "alb_dns_name" {
  description = "ALB DNS name — use this as your Locust target host"
  value       = aws_lb.this.dns_name
}

output "alb_arn" {
  value = aws_lb.this.arn
}

output "waiting_room_target_group_arn" {
  value = aws_lb_target_group.waiting_room.arn
}

output "flash_sale_api_target_group_arn" {
  value = aws_lb_target_group.flash_sale_api.arn
}
