.PHONY: protos

# This command generates all your protobuf files.
protos:
	@echo "--- Generating Protobuf Go files ---"
	@protoc \
		--proto_path=protos \
		--go_out=. \
		--go_opt=module=ig-wva \
		--go-grpc_out=. \
		--go-grpc_opt=module=ig-wva \
		$(find protos -name "*.proto")
	@echo "--- Done ---"