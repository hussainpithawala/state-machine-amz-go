failed=0
passed=0

while IFS= read -r -d '' file; do
  if go run "$file" < /dev/null > /dev/null 2>&1; then
    echo "✅ PASS: $file"
    ((passed++))
  else
    echo "❌ FAIL: $file (exit: $?)"
    ((failed++))
  fi
done < <(
  find examples -maxdepth 2 -type f \
    ! -path '*/distributed_queue/*' \
    ! -path '*/message_timeout_complete/*' \
    ! -path '*/test_tls_conn/*' \
    ! -path '*/test_secure_redis/*' \
    -name "*.go" -print0
)

echo "-----------------------------------"
echo "Summary: ✅ $passed passed, ❌ $failed failed"

exit $failed