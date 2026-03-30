output "orders_table_name" {
  value = aws_dynamodb_table.orders.name
}

output "orders_table_arn" {
  value = aws_dynamodb_table.orders.arn
}

output "inventory_table_name" {
  value = aws_dynamodb_table.inventory.name
}
