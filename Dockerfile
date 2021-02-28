FROM alpine
COPY codesearch /codesearch
EXPOSE 8000
ENV CODESEARCH_VOLUME=/db
CMD ["/codesearch"]
