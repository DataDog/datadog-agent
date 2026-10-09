from datadog_checks.base import AgentCheck


class HistogramLabCheck(AgentCheck):
    __NAMESPACE__ = 'repro'

    def check(self, instance):
        # Non-monotonic huge count: one key (equal bounds), 1.42e12 observations.
        # This is the smallest input that triggers the overflow-bin allocation
        # path via the Python check route.
        self.submit_histogram_bucket(
            'histogram_lab.bucket',
            1420000000000,  # count (int)
            0.0,            # lower_bound
            0.0,            # upper_bound (equal → one key)
            False,          # monotonic=False → raw count, no delta
            None,           # hostname
            ['case:non_monotonic_huge'],
        )

        # Optional monotonic baseline+jump: two runs with increasing raw values.
        # Run with --check-times 2; first call establishes baseline=1, second
        # jumps to 1 + 1.42e12, exercising the delta path.
        if instance.get('monotonic_jump', False):
            raw = 1 if self._get_state('baseline') is None else 1 + 1420000000000
            self._set_state('baseline', raw)
            self.submit_histogram_bucket(
                'histogram_lab.monotonic',
                raw,
                0.0,
                0.0,
                True,  # monotonic=True → delta computed
                None,
                ['case:monotonic_jump'],
            )
