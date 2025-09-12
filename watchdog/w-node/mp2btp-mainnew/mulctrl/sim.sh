#!/bin/bash

# 파라미터 셋 정의
symbol_sizes=(1024) # 32 64 128 256 512 
num_symbols=(16) #(128 192 256 320 384 448 512) # 16 32 

# code_rates: 0.00 ~ 0.30 (0.01 간격)
code_rates=()
for i in $(seq 0 70); do
  code_rates+=("$(printf "%.2f" "$(echo "$i * 0.01" | bc)")")
done

# loss_rates: 0.00 ~ 0.10 (0.01 간격)
loss_rates=()
for i in $(seq 0 10); do
  loss_rates+=("$(printf "%.2f" "$(echo "$i * 0.01" | bc)")")
done

file_name="../example/image.jpg"
iterations=1000

log_dir="./logs"
mkdir -p "$log_dir"
rm "$log_dir"/*

echo "Start Simulation! "
echo "----------------------------------------------------------------------"

# 모든 조합에 대해 시뮬레이션 수행
for s in "${symbol_sizes[@]}"; do
  for k in "${num_symbols[@]}"; do
    for l in "${loss_rates[@]}"; do
      for c in "${code_rates[@]}"; do
        log_file="$log_dir/s${s}_k${k}_c${c}_l${l}.log"
        echo "Running: ./raptor_sim -s $s -k $k -c $c -l $l -f $file_name -i $iterations"
        ./raptor_sim -s "$s" -k "$k" -c "$c" -l "$l" -f "$file_name" -i "$iterations" > "$log_file" 2>&1

        filename=$(basename "$log_file")
        s=$(echo "$filename" | sed -n 's/^s\([0-9]\+\)_k[0-9]\+_c[0-9.]\+_l[0-9.]\+\.log$/\1/p')
        k=$(echo "$filename" | sed -n 's/^s[0-9]\+_k\([0-9]\+\)_c[0-9.]\+_l[0-9.]\+\.log$/\1/p')
        c=$(echo "$filename" | sed -n 's/^s[0-9]\+_k[0-9]\+_c\([0-9.]\+\)_l[0-9.]\+\.log$/\1/p')
        l=$(echo "$filename" | sed -n 's/^s[0-9]\+_k[0-9]\+_c[0-9.]\+_l\([0-9.]\+\)\.log$/\1/p')
        
        if grep -q "Simulation Success!" "$log_file"; then
          echo "✅ Success: symbol size = $s, num symbols = $k, code rate = $c, loss rate = $l"  
          break
        else
          echo "❌ Failure: symbol size = $s, num symbols = $k, code rate = $c, loss rate = $l"
          rm "$log_file"
        fi
      done
    done
  done
done
