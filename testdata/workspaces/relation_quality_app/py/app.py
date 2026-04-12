from py.worker import process_data


class Handler:
    def run(self, enabled):
        if enabled:
            return self.finish(process_data())
        return process_data()

    def finish(self, value):
        return value


def handle_request(enabled):
    if enabled:
        return process_data()
    return process_data()
