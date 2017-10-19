#!/usr/bin/env ruby

gem 'pg'
require 'pg'

def insert(conn)
  puts("[#{Time.now}] Inserting...")
  conn.exec("insert into times (now())")
end

conn = PG.connect(dbname: 'postgres', host: '127.0.0.1', user: 'postgres')

loop do
  insert(conn)
  sleep(0.5)
end
