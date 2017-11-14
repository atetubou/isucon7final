# coding: utf-8
=begin
nginxのログファイルの解析

nginxの設定:
log_format  mainlog  '$status|$request_time|$msec|$request_length|$remote_addr|$remote_user|$time_local|$body_bytes_sent|$request|$http_referer|$http_user_agent|$http_x_forwarded_for|$connection';
access_log  /home/isucon/access.log  mainlog;

上の設定で/home/isucon/access.logにログファイルが出来るので、それを標準入力に流し込む

ログの内容: http://nginx.org/en/docs/http/ngx_http_log_module.html

Usage: ruby stat.rb [まとめたいURLのprefix] [prefix2...]  < access.log
例えば
ruby stat.rb  /memo /recent <access.log
とやると/memoとか/recentから始まるURLはまとまる。


$ ruby tools/stat.rb /diary/entry/ /diary/entries/ /diary/comment/ /profile/ /friends/
=end

def string_of(x)
  if x.class == Float then
    "%.4f" % x
  else
    x.to_s
  end
end

def get_url_of(h)
  status  = h[:status]
  url = h[:request].split(" ")[1]
  req = h[:request].split(" ")[0]
  ARGV.each do |s|
    if url[0..s.length-1] == s then
      url = s
      break
    end
  end
  "#{status} #{req.ljust(4)} #{url}"
end

log_data = []
while $stdin.gets
  keys = [:status, :request_time, :msec, :request_length, :remote_addr, :remote_user, :time_local, :body_bytes_sent, :request, :http_referer, :http_user_agent, :http_x_forwarded_for, :connection]
  values = $_.split("|")

  next if keys.length != values.length
  h = Hash[[keys, values].transpose]
  log_data.push h
end


url2count = Hash.new(0)
url2time = Hash.new(0.0)
log_data.each do |h|
  url = get_url_of(h)
  url2count[url] += 1
  url2time[url] += h[:request_time].to_f
end

table = []
url2count.keys.each do |url|
  line = [url2time[url], url2count[url], url2time[url]/url2count[url], url]
  table.push line
end
table.sort! {|a, b| b <=> a}


header = ["time", "count", "time/count", "URL"]
table = [header] + table
lengths = table.transpose.map {|col| col.map{|x| string_of(x).length }.max }

table.each do |line|
  puts [lengths, line].transpose.map{|len, x| string_of(x).ljust(len)}.join("  ")
end
