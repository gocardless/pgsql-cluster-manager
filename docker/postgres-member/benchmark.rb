#!/usr/bin/env ruby

def benchmark_write(size_mb, block_size)
  count = (size_mb * 1024 * 1024) / block_size

  system("rm -f /data/benchmark_file")
  system("sync")

  start = Time.now
  system("dd if=/dev/zero of=/data/benchmark_file bs=#{block_size} count=#{count} conv=fdatasync")
  system("sync")

  Time.now - start
end

block_sizes = [256, 512, 1024, 2048, 4096, 8192]
block_sizes.each do |bs|
  puts([`hostname`.chomp, bs, benchmark_write(5000, bs)].join("\t"))
end
