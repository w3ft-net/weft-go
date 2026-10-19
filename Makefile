conformance:
	rm -f testdata/conformance/*.json
	cp ../sdk-conformance-specs/*.json testdata/conformance/
	git -C ../sdk-conformance-specs rev-parse HEAD > testdata/conformance/VERSION

.PHONY: conformance
