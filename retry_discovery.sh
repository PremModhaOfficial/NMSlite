#!/bin/bash
LOGIN_RESP=$(curl -s -X POST http://localhost:8080/api/v1/login -H "Content-Type: application/json" -d '{"username":"admin", "password":"Admin@123"}')
TOKEN=$(echo $LOGIN_RESP | python3 -c "import sys, json; print(json.load(sys.stdin)['token'])")

for i in {1..6}; do
  echo "Attempt $i: Triggering discovery..."
  curl -s -X POST http://localhost:8080/api/v1/discoveries/1/run -H "Authorization: Bearer $TOKEN" > /dev/null
  
  sleep 15
  
  STATUS=$(curl -s -X GET http://localhost:8080/api/v1/discoveries/1 -H "Authorization: Bearer $TOKEN")
  RUN_STATUS=$(echo $STATUS | python3 -c "import sys, json; print(json.load(sys.stdin).get('last_run_status', ''))")
  DEVICES=$(echo $STATUS | python3 -c "import sys, json; print(json.load(sys.stdin).get('devices_discovered', 0))")
  
  echo "Status: $RUN_STATUS, Devices: $DEVICES"
  
  if [ "$DEVICES" -gt 0 ]; then
    echo "Success! Device discovered."
    break
  fi
done

# Check Monitors
echo "Monitors:"
curl -s -X GET http://localhost:8080/api/v1/monitors -H "Authorization: Bearer $TOKEN"
