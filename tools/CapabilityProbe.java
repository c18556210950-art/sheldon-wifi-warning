import android.content.Context;
import android.os.Process;
import java.lang.reflect.Method;

/** Read-only shell-UID probe. Does not create a network or change hotspot state. */
public class CapabilityProbe {
    public static void main(String[] args) throws Exception {
        android.os.Looper.prepareMainLooper();
        System.out.println("uid=" + Process.myUid());
        Class<?> activity = Class.forName("android.app.ActivityThread");
        Object thread = activity.getMethod("systemMain").invoke(null);
        Context context = (Context) activity.getMethod("getSystemContext").invoke(thread);
        for (String permission : new String[]{"MANAGE_TEST_NETWORKS", "TETHER_PRIVILEGED", "NETWORK_SETTINGS"}) {
            System.out.println(permission + "=" + context.checkSelfPermission("android.permission." + permission));
        }
        Object test = context.getSystemService("test_network");
        Object tether = context.getSystemService("tethering");
        System.out.println("test_network=" + (test != null));
        System.out.println("tethering=" + (tether != null));
        if (test != null) for (Method m : test.getClass().getMethods()) {
            if (m.getName().equals("createTunInterface") || m.getName().equals("setupTestNetwork"))
                System.out.println(m.toString());
        }
        if (tether != null) {
            System.out.println(tether.getClass().getMethod("setPreferTestNetworks", boolean.class));
        }
        System.out.println("READ_ONLY_PROBE_COMPLETE");
        if (args.length > 0 && args[0].equals("--tun")) {
            Object tun = test.getClass().getMethod("createTunInterface", java.util.Collection.class).invoke(test,
                java.util.Collections.singletonList(Class.forName("android.net.LinkAddress").getConstructor(java.net.InetAddress.class, int.class).newInstance(java.net.InetAddress.getByName("192.0.2.2"), 24)));
            android.os.ParcelFileDescriptor fd = (android.os.ParcelFileDescriptor) tun.getClass().getMethod("getFileDescriptor").invoke(tun);
            try {
                System.out.println("TUN_CREATED=" + tun.getClass().getMethod("getInterfaceName").invoke(tun));
            } finally {
                fd.close();
                System.out.println("TUN_FD_CLOSED");
            }
        }
        System.exit(0);
    }
}
