from gremlins import faults, metafaults, triggers, tc
clear_network_faults = faults.clear_network_faults()
introduce_partition = faults.introduce_network_partition()
introduce_latency = faults.introduce_network_latency()

profile = [
    triggers.OneShot(clear_network_faults),
    triggers.Periodic(
        30, metafaults.pick_fault([
            (3, clear_network_faults),
            (12, introduce_partition),
        ])),
]
