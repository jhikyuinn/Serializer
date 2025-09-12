#!/bin/bash

log_dir="./logs"

# CSV 헤더 출력
echo "symbol_size,num_symbols,code_rate,loss_rate,encoding_time,decoding_time,enc_throughput,dec_throughput"

for file in "$log_dir"/*.log; do
  filename=$(basename "$file")

  # 파일명에서 s, k, c, l 값 추출
  s=$(echo "$filename" | sed -n 's/^s\([0-9]\+\)_k.*$/\1/p')
  k=$(echo "$filename" | sed -n 's/^s[0-9]\+_k\([0-9]\+\)_c.*$/\1/p')
  c=$(echo "$filename" | sed -n 's/^.*_c\([0-9.]\+\)_l.*$/\1/p')
  l=$(echo "$filename" | sed -n 's/^.*_l\([0-9.]\+\)\.log$/\1/p')

  # [ENC], [DEC] 시간 추출
  enc_time=$(grep "\[ENC\]" "$file" | awk '{print $NF}')
  dec_time=$(grep "\[DEC\]" "$file" | awk '{print $NF}')

  enc_time_avg=$(echo "$enc_time" | awk '{sum += $1} END {if (NR > 0) print sum / NR; else print 0}')
  dec_time_avg=$(echo "$dec_time" | awk '{sum += $1} END {if (NR > 0) print sum / NR; else print 0}')

  if (( $(echo "$enc_time_avg > 0" | bc -l) )) && (( $(echo "$dec_time_avg > 0" | bc -l) )); then
    raw_enc_throughput=$(echo "$s * $k / $enc_time_avg" | bc -l)
    raw_dec_throughput=$(echo "$s * $k / $dec_time_avg" | bc -l)

    # 반올림: 소수점 둘째 자리까지
    enc_throughput=$(printf "%.2f" "$raw_enc_throughput")
    dec_throughput=$(printf "%.2f" "$raw_dec_throughput")
  else
    enc_throughput="NaN"
    dec_throughput="NaN"
  fi

  echo "$s,$k,$c,$l,$enc_time_avg,$dec_time_avg,$enc_throughput,$dec_throughput"
done
